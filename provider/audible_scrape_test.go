package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"

	"github.com/prairie-server/prairie-plugin-metadata-audiobook/metadata"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestAudibleParseProductPage(t *testing.T) {
	fixture, err := os.ReadFile("testdata/audible_product.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(fixture))
	if err != nil {
		t.Fatalf("parse HTML: %v", err)
	}

	scraper := NewAudibleScraper()
	m := scraper.parseProductPage(doc, "B002V0QHBU")
	if m == nil {
		t.Fatal("expected non-nil match")
	}

	if m.Provider != "audible" {
		t.Errorf("Provider = %q, want audible", m.Provider)
	}
	if m.ASIN != "B002V0QHBU" {
		t.Errorf("ASIN = %q", m.ASIN)
	}
	// Title should have series book suffix stripped and be split at colon if any.
	if m.Title == "" {
		t.Error("Title should not be empty")
	}
	if len(m.Authors) == 0 || m.Authors[0] != "Douglas Adams" {
		t.Errorf("Authors = %v", m.Authors)
	}
	if len(m.Narrators) == 0 || m.Narrators[0] != "Stephen Fry" {
		t.Errorf("Narrators = %v", m.Narrators)
	}
	if m.Publisher != "Macmillan Audio" {
		t.Errorf("Publisher = %q", m.Publisher)
	}
	if m.PublishYear != 2005 {
		t.Errorf("PublishYear = %d", m.PublishYear)
	}
	// ISO duration PT3H13M = 3*60+13 = 193 minutes.
	if m.DurationMin != 193 {
		t.Errorf("DurationMin = %d, want 193", m.DurationMin)
	}
	// "Home" should be filtered from breadcrumb genres.
	for _, g := range m.Genres {
		if g == "Home" {
			t.Errorf("Genres should not include 'Home': %v", m.Genres)
		}
	}
	if len(m.Genres) < 1 {
		t.Errorf("expected at least one genre, got %v", m.Genres)
	}
	// Series from the <a href="/series/..."> link.
	if m.SeriesName == "" {
		t.Error("SeriesName should not be empty")
	}
	if m.SeriesPosition != "1" {
		t.Errorf("SeriesPosition = %q, want 1", m.SeriesPosition)
	}
}

func TestAudibleParseProductPageNilDocument(t *testing.T) {
	scraper := NewAudibleScraper()
	if match := scraper.parseProductPage(nil, "B002V0QHBU"); match != nil {
		t.Fatalf("parseProductPage(nil) = %#v, want nil", match)
	}
}

func TestAudibleParseProductPageSingleJSONLDAndNoBook(t *testing.T) {
	html := `<html><body>
<script type="application/ld+json">{
  "@type": "BreadcrumbList",
  "itemListElement": [{"item":{"name":"Home"}},{"item":{"name":"Mystery"}}]
}</script>
<script type="application/ld+json">{
  "@type":"Audiobook",
  "name":"Standalone Title",
  "description":"<p>Desc</p>",
  "image":"https://cdn.example.test/cover.jpg",
  "inLanguage":"en",
  "datePublished":"2022-04-05",
  "duration":"PT45M30S",
  "author":{"name":"Solo Author"},
  "readBy":[{"name":"Narrator One"},{"name":""}],
  "publisher":"Example Audio"
}</script>
<a href="/series/example">Example Series</a>
</body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	match := NewAudibleScraper().parseProductPage(doc, "b002v0qhbu")
	if match == nil {
		t.Fatal("expected match")
	}
	if match.Title != "Standalone Title" || match.Subtitle != "" {
		t.Fatalf("title/subtitle = %q/%q", match.Title, match.Subtitle)
	}
	if match.DurationMin != 45 || match.PublishYear != 2022 {
		t.Fatalf("duration/year = %d/%d", match.DurationMin, match.PublishYear)
	}
	if len(match.Genres) != 1 || match.Genres[0] != "Mystery" {
		t.Fatalf("genres = %#v", match.Genres)
	}
	if match.SeriesName != "Example Series" || match.SeriesPosition != "" {
		t.Fatalf("series = %q/%q", match.SeriesName, match.SeriesPosition)
	}

	noBook, err := goquery.NewDocumentFromReader(strings.NewReader(`<script type="application/ld+json">{"@type":"WebPage"}</script>`))
	if err != nil {
		t.Fatal(err)
	}
	if got := NewAudibleScraper().parseProductPage(noBook, "B002V0QHBU"); got != nil {
		t.Fatalf("parseProductPage(no audiobook) = %#v, want nil", got)
	}

	colonDoc, err := goquery.NewDocumentFromReader(strings.NewReader(`<script type="application/ld+json">{
  "@type":"Audiobook","name":"Main Title: The Subtitle","author":{"name":"A"}
}</script><a href="/series/blank"></a>`))
	if err != nil {
		t.Fatal(err)
	}
	colon := NewAudibleScraper().parseProductPage(colonDoc, "B002V0QHBU")
	if colon == nil || colon.Title != "Main Title" || colon.Subtitle != "The Subtitle" {
		t.Fatalf("colon title = %#v", colon)
	}
}

func TestAudibleFetchNotFoundReturnsNil(t *testing.T) {
	scraper := NewAudibleScraper()
	scraper.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		}),
	}

	match, err := scraper.Fetch(context.Background(), "B002V0QHBU")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if match != nil {
		t.Fatalf("Fetch() = %#v, want nil", match)
	}
}

// Search reads the results page cards directly; it must not fetch product
// pages, since each page costs a token from the six-per-minute bucket.
func TestAudibleSearchReadsCardsWithoutProductPages(t *testing.T) {
	fixture, err := os.ReadFile("testdata/audible_search_cards.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var productHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			productHits++
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("keywords"); got != "Ready Player One Ernest Cline" {
			t.Errorf("keywords = %q", got)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(fixture)
	}))
	defer srv.Close()

	scraper := NewAudibleScraper()
	scraper.httpClient = srv.Client()
	scraper.httpClient.Transport = rewriteHost(srv.URL, scraper.httpClient.Transport)

	results, err := scraper.Search(context.Background(), metadata.SearchQuery{Title: "Ready Player One", Authors: []string{"Ernest Cline"}})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if productHits != 0 {
		t.Fatalf("product page requests = %d, want 0", productHits)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	first := results[0]
	if first.ASIN != "B005FRGT44" || first.ProviderID != "B005FRGT44" || first.Provider != "audible" {
		t.Errorf("first identity = %+v", first)
	}
	if first.Title != "Ready Player One" {
		t.Errorf("Title = %q", first.Title)
	}
	if len(first.Authors) != 1 || first.Authors[0] != "Ernest Cline" {
		t.Errorf("Authors = %v", first.Authors)
	}
	if len(first.Narrators) != 1 || first.Narrators[0] != "Wil Wheaton" {
		t.Errorf("Narrators = %v", first.Narrators)
	}
	if first.DurationMin != 940 {
		t.Errorf("DurationMin = %d, want 940", first.DurationMin)
	}
	if first.CoverURL != "https://m.media-amazon.com/images/I/41Eptolyo+L._SL500_.jpg" {
		t.Errorf("CoverURL = %q", first.CoverURL)
	}
	if got := results[1].Narrators; len(got) != 2 || got[1] != "Jane Doe" {
		t.Errorf("second Narrators = %v", got)
	}
	if results[2].DurationMin != 45 {
		t.Errorf("minutes-only length = %d, want 45", results[2].DurationMin)
	}
}

func TestParseSearchResultsHonorsLimit(t *testing.T) {
	fixture, err := os.ReadFile("testdata/audible_search_cards.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(fixture))
	if err != nil {
		t.Fatalf("parse HTML: %v", err)
	}
	if got := len(parseSearchResults(doc, 2)); got != 2 {
		t.Fatalf("results = %d, want 2", got)
	}
}

func TestParseAudibleLength(t *testing.T) {
	tests := map[string]int{"15 hrs and 40 mins": 940, "1 hr and 1 min": 61, "45 mins": 45, "2 hrs": 120, "": 0}
	for in, want := range tests {
		if got := parseAudibleLength(in); got != want {
			t.Errorf("parseAudibleLength(%q) = %d, want %d", in, got, want)
		}
	}
}

// rewriteHost points every request at the test server so the scraper's
// hard-coded audible.com base URL is exercised unchanged.
func rewriteHost(target string, next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = "http"
		clone.URL.Host = strings.TrimPrefix(target, "http://")
		clone.Host = clone.URL.Host
		return next.RoundTrip(clone)
	})
}

func TestParseISODurationToMin(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"PT3H13M", 193},
		{"PT10H30M", 630},
		{"PT45M", 45},
		{"PT1H", 60},
		{"PT1H30M45S", 90},
		{"", 0},
		{"garbage", 0},
	}
	for _, tt := range tests {
		got := parseISODurationToMin(tt.in)
		if got != tt.want {
			t.Errorf("parseISODurationToMin(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestAudibleJSONLDPersonNamesInvalidShapes(t *testing.T) {
	if got := extractJSONLDPersonNames(nil); got != nil {
		t.Fatalf("nil names = %#v", got)
	}
	if got := extractJSONLDPersonNames(json.RawMessage(`{"name":""}`)); got != nil {
		t.Fatalf("empty single name = %#v", got)
	}
	if got := extractJSONLDPersonNames(json.RawMessage(`not-json`)); got != nil {
		t.Fatalf("invalid names = %#v", got)
	}
	if got := extractJSONLDPersonNames(json.RawMessage(`[{"name":""},{"name":"Kept"}]`)); len(got) != 1 || got[0] != "Kept" {
		t.Fatalf("filtered names = %#v", got)
	}
	if got := parseDigits("12x"); got != 0 {
		t.Fatalf("parseDigits invalid = %d, want 0", got)
	}
}

func TestCleanSearchTerm(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Hello, World!", "Hello World"},
		{"It's a test", "Its a test"},
		{"normal words", "normal words"},
		{"  spaces  ", "spaces"},
	}
	for _, tt := range tests {
		got := cleanSearchTerm(tt.in)
		if got != tt.want {
			t.Errorf("cleanSearchTerm(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
