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

const audiotekaBaseURL = "https://audioteka.com"

var audiotekaRegionURLs = map[string]string{
	"pl": "https://audioteka.com/pl",
	"cz": "https://audioteka.com/cz",
	"de": "https://audioteka.com/de",
	"es": "https://audioteka.com/es",
	"lt": "https://audioteka.com/lt",
}

var audiotekaSearchPaths = map[string]string{
	"pl": "szukaj",
	"cz": "hledat",
	"de": "suchen",
	"es": "buscar",
	"lt": "ieškoti",
}

// AudiotekaScraper scrapes Audioteka search and detail pages.
type AudiotekaScraper struct {
	httpClient *http.Client
	baseURL    string
	limiter    *rate.Limiter
	region     string
}

func NewAudiotekaScraper() *AudiotekaScraper {
	return &AudiotekaScraper{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    audiotekaBaseURL,
		limiter:    newLimiter(6),
		region:     "pl",
	}
}

func (a *AudiotekaScraper) Search(ctx context.Context, q metadata.SearchQuery) ([]metadata.Match, error) {
	if q.Title == "" || isLikelyAudibleASIN(q.Title) {
		return nil, nil
	}
	if err := waitForLimiter(ctx, a.limiter); err != nil {
		return nil, err
	}
	searchPath := audiotekaSearchPaths[a.region]
	if searchPath == "" {
		searchPath = "szukaj"
	}
	searchURL := fmt.Sprintf("%s/%s?query=%s", a.host(), searchPath, url.QueryEscape(q.Title))
	body, err := a.fetch(ctx, searchURL)
	if err != nil || body == "" {
		return nil, err
	}
	return parseAudiotekaSearch(body), nil
}

func (a *AudiotekaScraper) Fetch(ctx context.Context, externalID string) (*metadata.Match, error) {
	externalID = strings.TrimSpace(externalID)
	if externalID == "" || isLikelyAudibleASIN(externalID) {
		return nil, nil
	}
	if err := waitForLimiter(ctx, a.limiter); err != nil {
		return nil, err
	}
	bookURL := fmt.Sprintf("%s/audiobook/%s", a.host(), url.PathEscape(externalID))
	body, err := a.fetch(ctx, bookURL)
	if err != nil || body == "" {
		return nil, err
	}
	match := parseAudiotekaBookPage(body)
	if match != nil && match.ProviderID == "" {
		match.ProviderID = externalID
	}
	return match, nil
}

func (a *AudiotekaScraper) host() string {
	if a.baseURL != audiotekaBaseURL {
		return a.baseURL
	}
	if host, ok := audiotekaRegionURLs[a.region]; ok {
		return host
	}
	return audiotekaRegionURLs["pl"]
}

func (a *AudiotekaScraper) fetch(ctx context.Context, fetchURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return "", fmt.Errorf("audioteka: create request: %w", err)
	}
	req.Header.Set("User-Agent", "silo-audiobook-metadata/audioteka")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("audioteka: request: %w", err)
	}
	return readLimitedBody(resp, "audioteka")
}

func parseAudiotekaBookPage(html string) *metadata.Match {
	if match := parseJSONLDMatch(html, "audioteka"); match != nil {
		return match
	}
	results := extractAudiotekaInitialState(html)
	if len(results) == 0 {
		return nil
	}
	return &results[0]
}

func parseAudiotekaSearch(html string) []metadata.Match {
	results := extractAudiotekaInitialState(html)
	if len(results) == 0 {
		if match := parseJSONLDMatch(html, "audioteka"); match != nil {
			return []metadata.Match{*match}
		}
	}
	if len(results) > 20 {
		return results[:20]
	}
	return results
}

func extractAudiotekaInitialState(html string) []metadata.Match {
	m := audiotekaInitialStateRE.FindStringSubmatch(html)
	if len(m) < 2 {
		return nil
	}
	var data interface{}
	if err := json.Unmarshal([]byte(m[1]), &data); err != nil {
		return nil
	}
	var out []metadata.Match
	traverseAudiotekaData(data, &out, 0)
	return out
}

func traverseAudiotekaData(v interface{}, out *[]metadata.Match, depth int) {
	if depth > maxTraverseDepth {
		return
	}
	switch val := v.(type) {
	case []interface{}:
		for _, item := range val {
			traverseAudiotekaData(item, out, depth+1)
		}
	case map[string]interface{}:
		if isAudiotekaBook(val) {
			if match := audiotekaMapToMatch(val); match != nil {
				*out = append(*out, *match)
			}
		}
		for _, child := range val {
			traverseAudiotekaData(child, out, depth+1)
		}
	}
}

func isAudiotekaBook(m map[string]interface{}) bool {
	_, hasTitle := m["title"]
	if !hasTitle {
		return false
	}
	_, hasID := m["id"]
	_, hasAuthors := m["authors"]
	_, hasNarrators := m["narrators"]
	_, hasDuration := m["duration"]
	_, hasCoverURL := m["coverUrl"]
	_, hasISBN := m["isbn"]
	return hasID || hasAuthors || hasNarrators || hasDuration || hasCoverURL || hasISBN
}

func audiotekaMapToMatch(m map[string]interface{}) *metadata.Match {
	match := &metadata.Match{Provider: "audioteka"}
	match.Title = stringField(m, "title")
	if match.Title == "" {
		return nil
	}
	match.ProviderID = stringField(m, "id")
	match.Description = stringField(m, "description")
	match.Language = stringField(m, "language")
	match.ISBN = stringField(m, "isbn")
	match.Publisher = stringField(m, "publisher")
	match.PublishYear = extractYear(stringField(m, "releaseDate"))
	if dur, ok := m["duration"].(float64); ok && dur > 0 {
		match.DurationMin = int(dur) / 60
	}
	match.CoverURL = stringField(m, "coverUrl")
	match.Authors = namesFromArray(m["authors"])
	match.Narrators = namesFromArray(m["narrators"])
	if series, ok := m["series"].(map[string]interface{}); ok {
		match.SeriesName = stringField(series, "name")
		if pos, ok := series["orderInSeries"].(float64); ok {
			match.SeriesPosition = numberPosition(pos)
		}
	}
	for _, key := range []string{"categories", "genres", "kategorie"} {
		if len(match.Genres) == 0 {
			match.Genres = stringArrayFromField(m[key])
		}
	}
	return match
}
