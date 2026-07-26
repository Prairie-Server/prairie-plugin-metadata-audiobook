package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/prairie-server/prairie-plugin-metadata-audiobook/metadata"
)

// StorytelScraper scrapes Storytel for audiobook metadata.
//
// Reference implementation:
//
//	/opt/booklore-ng/src/lib/metadata/providers/storytel.ts
//
// Storytel embeds book data as Next.js hydration JSON (__NEXT_DATA__) or
// as JSON-LD. We prefer __NEXT_DATA__ for search results and JSON-LD for
// individual book pages.
//
// Rate limit: 6 rpm (very conservative scraping).
type StorytelScraper struct {
	httpClient *http.Client
	limiter    *rate.Limiter
	region     string
}

// storytelRegionDomain maps region codes to Storytel domain fragments.
var storytelRegionDomain = map[string]string{
	"us": "com",
	"uk": "co.uk",
	"se": "se",
	"no": "no",
	"dk": "dk",
	"fi": "fi",
	"nl": "nl",
	"be": "be",
	"de": "de",
	"es": "es",
	"it": "it",
	"pl": "pl",
	"ru": "ru",
	"tr": "com.tr",
	"in": "in",
	"br": "com.br",
	"mx": "mx",
	"ae": "ae",
	"bg": "bg",
	"eg": "eg",
	"is": "is",
	"kr": "kr",
	"ro": "ro",
	"sg": "sg",
	"th": "th",
}

var (
	nextDataRE = regexp.MustCompile(`(?i)<script[^>]*id="__NEXT_DATA__"[^>]*>([\s\S]*?)</script>`)
	jsonLdRE   = regexp.MustCompile(`(?i)<script[^>]*type="application/ld\+json"[^>]*>([\s\S]*?)</script>`)
)

// NewStorytelScraper creates a Storytel scraper for the US region with 6 rpm limit.
func NewStorytelScraper() *StorytelScraper {
	return &StorytelScraper{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		limiter:    newLimiter(6),
		region:     "us",
	}
}

func (s *StorytelScraper) domain() string {
	if d, ok := storytelRegionDomain[s.region]; ok {
		return "storytel." + d
	}
	return "storytel.com"
}

// Search fetches the Storytel search page and extracts book matches.
func (s *StorytelScraper) Search(ctx context.Context, q metadata.SearchQuery) ([]metadata.Match, error) {
	if q.Title == "" {
		return nil, nil
	}

	if err := waitForLimiter(ctx, s.limiter); err != nil {
		return nil, err
	}

	searchURL := "https://www." + s.domain() + "/search?query=" + url.QueryEscape(q.Title)
	body, err := s.fetch(ctx, searchURL)
	if err != nil || body == "" {
		return nil, err
	}

	return parseStorytelSearch(body), nil
}

// Fetch retrieves a single Storytel book by its ID/slug.
func (s *StorytelScraper) Fetch(ctx context.Context, bookID string) (*metadata.Match, error) {
	if err := waitForLimiter(ctx, s.limiter); err != nil {
		return nil, err
	}

	bookURL := "https://www." + s.domain() + "/books/" + url.PathEscape(bookID)
	body, err := s.fetch(ctx, bookURL)
	if err != nil || body == "" {
		return nil, err
	}

	return parseStorytelBookPage(body), nil
}

// storytelBook is the rich book shape found in __NEXT_DATA__.
type storytelBook struct {
	ConsumableID string `json:"consumableId"`
	BookID       string `json:"bookId"`
	Title        string `json:"title"`
	Slogan       string `json:"slogan"`
	Description  string `json:"description"`
	Authors      []struct {
		Name string `json:"name"`
	} `json:"authors"`
	Narrators []struct {
		Name string `json:"name"`
	} `json:"narrators"`
	Publisher   string  `json:"publisher"`
	ReleaseDate string  `json:"releaseDate"`
	Duration    float64 `json:"duration"`
	Language    string  `json:"language"`
	ISBN        string  `json:"isbn"`
	Cover       *struct {
		URL   string `json:"url"`
		Sizes []struct {
			Width  int    `json:"width"`
			Height int    `json:"height"`
			URL    string `json:"url"`
		} `json:"sizes"`
	} `json:"cover"`
	Series *struct {
		Name          string  `json:"name"`
		OrderInSeries float64 `json:"orderInSeries"`
	} `json:"series"`
	Categories []string `json:"categories"`
}

func matchFromStorytelBook(b storytelBook) metadata.Match {
	authors := make([]string, 0, len(b.Authors))
	for _, a := range b.Authors {
		if a.Name != "" {
			authors = append(authors, a.Name)
		}
	}

	narrators := make([]string, 0, len(b.Narrators))
	for _, n := range b.Narrators {
		if n.Name != "" {
			narrators = append(narrators, n.Name)
		}
	}

	coverURL := ""
	if b.Cover != nil {
		if len(b.Cover.Sizes) > 0 {
			// Pick largest by width.
			best := b.Cover.Sizes[0]
			for _, sz := range b.Cover.Sizes[1:] {
				if sz.Width > best.Width {
					best = sz
				}
			}
			coverURL = best.URL
		} else {
			coverURL = b.Cover.URL
		}
	}

	id := b.ConsumableID
	if id == "" {
		id = b.BookID
	}

	var seriesName, seriesPos string
	if b.Series != nil {
		seriesName = b.Series.Name
		if b.Series.OrderInSeries != 0 {
			seriesPos = fmt.Sprintf("%v", b.Series.OrderInSeries)
		}
	}

	durationMin := 0
	if b.Duration > 0 {
		// Storytel duration is in seconds.
		durationMin = int(b.Duration / 60)
	}

	return metadata.Match{
		Provider:       "storytel",
		ProviderID:     id,
		ISBN:           b.ISBN,
		Title:          b.Title,
		Subtitle:       b.Slogan,
		Authors:        authors,
		Narrators:      narrators,
		Publisher:      b.Publisher,
		PublishYear:    extractYear(b.ReleaseDate),
		DurationMin:    durationMin,
		Description:    b.Description,
		CoverURL:       coverURL,
		Language:       b.Language,
		Genres:         b.Categories,
		SeriesName:     seriesName,
		SeriesPosition: seriesPos,
	}
}

// parseStorytelSearch extracts book matches from a Storytel search page HTML.
func parseStorytelSearch(html string) []metadata.Match {
	// 1. Try __NEXT_DATA__ (Next.js hydration data).
	if m := nextDataRE.FindStringSubmatch(html); len(m) == 2 {
		var data interface{}
		if err := json.Unmarshal([]byte(m[1]), &data); err == nil {
			books := extractStorytelBooksFromNextData(data)
			if len(books) > 20 {
				books = books[:20]
			}
			if len(books) > 0 {
				results := make([]metadata.Match, 0, len(books))
				for _, b := range books {
					results = append(results, matchFromStorytelBook(b))
				}
				return results
			}
		}
	}

	// 2. Fallback: no reliable HTML structure without a full DOM parser.
	return nil
}

// parseStorytelBookPage extracts a single book from a Storytel book page.
func parseStorytelBookPage(html string) *metadata.Match {
	// 1. Try JSON-LD.
	if m := jsonLdRE.FindStringSubmatch(html); len(m) == 2 {
		var node struct {
			Type        string      `json:"@type"`
			Name        string      `json:"name"`
			Description string      `json:"description"`
			Image       string      `json:"image"`
			InLanguage  string      `json:"inLanguage"`
			DatePublished string    `json:"datePublished"`
			Duration    string      `json:"duration"`
			ISBN        string      `json:"isbn"`
			Author      interface{} `json:"author"`
			ReadBy      interface{} `json:"readBy"`
			Publisher   interface{} `json:"publisher"`
			AggregateRating *struct {
				RatingValue float64 `json:"ratingValue"`
				RatingCount int     `json:"ratingCount"`
			} `json:"aggregateRating"`
		}
		if err := json.Unmarshal([]byte(m[1]), &node); err == nil &&
			(node.Type == "Audiobook" || node.Type == "Book") {
			result := &metadata.Match{
				Provider:    "storytel",
				Title:       node.Name,
				Description: node.Description,
				CoverURL:    node.Image,
				Language:    node.InLanguage,
				ISBN:        node.ISBN,
				PublishYear: extractYear(node.DatePublished),
				DurationMin: parseISODurationToMin(node.Duration),
			}
			result.Authors = extractJSONLDNamesFromInterface(node.Author)
			result.Narrators = extractJSONLDNamesFromInterface(node.ReadBy)
			result.Publisher = extractPublisherName(node.Publisher)
			return result
		}
	}

	// 2. Try __NEXT_DATA__ as fallback.
	if m := nextDataRE.FindStringSubmatch(html); len(m) == 2 {
		var data interface{}
		if err := json.Unmarshal([]byte(m[1]), &data); err == nil {
			books := extractStorytelBooksFromNextData(data)
			if len(books) > 0 {
				match := matchFromStorytelBook(books[0])
				return &match
			}
		}
	}

	return nil
}

// extractStorytelBooksFromNextData traverses arbitrary Next.js page props JSON
// looking for objects that look like Storytel book records.
func extractStorytelBooksFromNextData(data interface{}) []storytelBook {
	var results []storytelBook

	var traverse func(v interface{})
	traverse = func(v interface{}) {
		switch node := v.(type) {
		case map[string]interface{}:
			// Heuristic: object has "title" and at least one book-indicator field.
			title, hasTitle := node["title"].(string)
			if hasTitle && title != "" {
				_, hasConsumable := node["consumableId"]
				_, hasBookID := node["bookId"]
				_, hasAuthors := node["authors"]
				_, hasNarrators := node["narrators"]
				_, hasDuration := node["duration"]
				if hasConsumable || hasBookID || hasAuthors || hasNarrators || hasDuration {
					// Marshal back to JSON and unmarshal into our typed struct.
					b, err := json.Marshal(node)
					if err == nil {
						var book storytelBook
						if err2 := json.Unmarshal(b, &book); err2 == nil {
							results = append(results, book)
							return // Don't recurse into a matched book node.
						}
					}
				}
			}
			for _, child := range node {
				traverse(child)
			}
		case []interface{}:
			for _, item := range node {
				traverse(item)
			}
		}
	}

	traverse(data)
	return results
}

// extractJSONLDNamesFromInterface handles author/readBy fields that may be a
// string, object with "name", or array of either.
func extractJSONLDNamesFromInterface(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case string:
		if val != "" {
			return []string{val}
		}
	case map[string]interface{}:
		if name, ok := val["name"].(string); ok && name != "" {
			return []string{name}
		}
	case []interface{}:
		var names []string
		for _, item := range val {
			switch x := item.(type) {
			case string:
				if x != "" {
					names = append(names, x)
				}
			case map[string]interface{}:
				if name, ok := x["name"].(string); ok && name != "" {
					names = append(names, name)
				}
			}
		}
		return names
	}
	return nil
}

// extractPublisherName handles publisher fields that may be a string or {"name":"..."}.
func extractPublisherName(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case map[string]interface{}:
		if name, ok := val["name"].(string); ok {
			return name
		}
	}
	return ""
}

func (s *StorytelScraper) fetch(ctx context.Context, fetchURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return "", fmt.Errorf("storytel: create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("storytel: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("storytel: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", fmt.Errorf("storytel: read response: %w", err)
	}
	return string(body), nil
}

// decodeHTMLEntities decodes common HTML entities in plain-text fields.
func decodeHTMLEntities(text string) string {
	r := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
		"&#x27;", "'",
		"&nbsp;", " ",
	)
	return r.Replace(text)
}
