package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/Silo-Server/silo-plugin-audiobook-metadata/metadata"
)

const bookBeatBaseURL = "https://www.bookbeat.com"

var bookBeatRegionURLs = map[string]string{
	"se": "https://www.bookbeat.com/se",
	"fi": "https://www.bookbeat.com/fi",
	"de": "https://www.bookbeat.de",
	"at": "https://www.bookbeat.at",
	"ch": "https://www.bookbeat.ch",
	"dk": "https://www.bookbeat.com/dk",
	"no": "https://www.bookbeat.com/no",
	"pl": "https://www.bookbeat.com/pl",
	"nl": "https://www.bookbeat.com/nl",
	"uk": "https://www.bookbeat.com/uk",
}

var bookBeatBookPaths = []string{"book", "bok", "buch", "boek"}

// BookBeatScraper scrapes BookBeat search and detail pages.
type BookBeatScraper struct {
	httpClient *http.Client
	baseURL    string
	limiter    *rate.Limiter
	region     string
}

func NewBookBeatScraper() *BookBeatScraper {
	return &BookBeatScraper{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    bookBeatBaseURL,
		limiter:    newLimiter(6),
		region:     "se",
	}
}

func (b *BookBeatScraper) Search(ctx context.Context, q metadata.SearchQuery) ([]metadata.Match, error) {
	if q.Title == "" || isLikelyAudibleASIN(q.Title) {
		return nil, nil
	}
	if err := waitForLimiter(ctx, b.limiter); err != nil {
		return nil, err
	}

	searchURL := fmt.Sprintf("%s/search?q=%s", b.host(), url.QueryEscape(q.Title))
	body, err := b.fetch(ctx, searchURL)
	if err != nil || body == "" {
		return nil, err
	}
	return parseBookBeatSearch(body), nil
}

func (b *BookBeatScraper) Fetch(ctx context.Context, externalID string) (*metadata.Match, error) {
	externalID = strings.TrimSpace(externalID)
	if externalID == "" || isLikelyAudibleASIN(externalID) {
		return nil, nil
	}
	if err := waitForLimiter(ctx, b.limiter); err != nil {
		return nil, err
	}

	for _, path := range bookBeatBookPaths {
		bookURL := fmt.Sprintf("%s/%s/%s", b.host(), path, url.PathEscape(externalID))
		body, err := b.fetch(ctx, bookURL)
		if err != nil {
			return nil, err
		}
		if body == "" {
			continue
		}
		match := parseBookBeatBookPage(body)
		if match != nil {
			if match.ProviderID == "" {
				match.ProviderID = externalID
			}
			return match, nil
		}
	}
	return nil, nil
}

func (b *BookBeatScraper) host() string {
	if b.baseURL != bookBeatBaseURL {
		return b.baseURL
	}
	if host, ok := bookBeatRegionURLs[b.region]; ok {
		return host
	}
	return bookBeatRegionURLs["se"]
}

func (b *BookBeatScraper) fetch(ctx context.Context, fetchURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return "", fmt.Errorf("bookbeat: create request: %w", err)
	}
	req.Header.Set("User-Agent", "silo-audiobook-metadata/bookbeat")
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("bookbeat: request: %w", err)
	}
	return readLimitedBody(resp, "bookbeat")
}

func parseBookBeatBookPage(html string) *metadata.Match {
	if match := parseJSONLDMatch(html, "bookbeat"); match != nil {
		return match
	}
	results := extractBookBeatNextData(html)
	if len(results) == 0 {
		return nil
	}
	return &results[0]
}

func parseBookBeatSearch(html string) []metadata.Match {
	results := extractBookBeatNextData(html)
	if len(results) > 20 {
		return results[:20]
	}
	return results
}

func extractBookBeatNextData(html string) []metadata.Match {
	m := nextDataRE.FindStringSubmatch(html)
	if len(m) < 2 {
		return nil
	}
	var data interface{}
	if err := json.Unmarshal([]byte(m[1]), &data); err != nil {
		return nil
	}
	var out []metadata.Match
	traverseBookBeatNextData(data, &out, 0)
	return out
}

func traverseBookBeatNextData(v interface{}, out *[]metadata.Match, depth int) {
	if depth > maxTraverseDepth {
		return
	}
	switch val := v.(type) {
	case []interface{}:
		for _, item := range val {
			traverseBookBeatNextData(item, out, depth+1)
		}
	case map[string]interface{}:
		if isBookBeatBook(val) {
			if match := bookBeatMapToMatch(val); match != nil {
				*out = append(*out, *match)
			}
		}
		for _, child := range val {
			traverseBookBeatNextData(child, out, depth+1)
		}
	}
}

func isBookBeatBook(m map[string]interface{}) bool {
	_, hasTitle := m["title"]
	if !hasTitle {
		return false
	}
	_, hasID := m["id"]
	_, hasBookID := m["bookId"]
	_, hasAuthors := m["authors"]
	_, hasNarrators := m["narrators"]
	_, hasDuration := m["duration"]
	_, hasCoverURL := m["coverUrl"]
	return hasID || hasBookID || hasAuthors || hasNarrators || hasDuration || hasCoverURL
}

func bookBeatMapToMatch(m map[string]interface{}) *metadata.Match {
	match := &metadata.Match{Provider: "bookbeat"}
	match.Title = stringField(m, "title")
	if match.Title == "" {
		return nil
	}
	match.ProviderID = stringField(m, "id")
	if match.ProviderID == "" {
		match.ProviderID = stringField(m, "bookId")
	}
	match.Description = stringField(m, "description")
	match.Language = stringField(m, "language")
	match.ISBN = stringField(m, "isbn")
	match.Publisher = stringField(m, "publisher")
	match.PublishYear = extractYear(firstNonEmpty(stringField(m, "releaseDate"), stringField(m, "publishDate")))
	if dur, ok := m["duration"].(float64); ok && dur > 0 {
		match.DurationMin = int(dur) / 60
	}
	match.CoverURL = bestCoverURL(m)
	match.Authors = namesFromArray(m["authors"])
	match.Narrators = namesFromArray(m["narrators"])
	if series, ok := m["series"].(map[string]interface{}); ok {
		match.SeriesName = stringField(series, "name")
		if pos, ok := series["orderInSeries"].(float64); ok {
			match.SeriesPosition = numberPosition(pos)
		}
	}
	match.Genres = stringArrayFromField(m["categories"])
	return match
}
