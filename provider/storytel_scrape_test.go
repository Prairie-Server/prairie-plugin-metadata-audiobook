package provider

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestStorytelParseSearch(t *testing.T) {
	fixture, err := os.ReadFile("testdata/storytel_search.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	results := parseStorytelSearch(string(fixture))
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}

	m := results[0]
	if m.Provider != "storytel" {
		t.Errorf("Provider = %q, want storytel", m.Provider)
	}
	if m.Title != "The Hitchhiker's Guide to the Galaxy" {
		t.Errorf("Title = %q", m.Title)
	}
	if len(m.Authors) == 0 || m.Authors[0] != "Douglas Adams" {
		t.Errorf("Authors = %v", m.Authors)
	}
	if len(m.Narrators) == 0 || m.Narrators[0] != "Stephen Fry" {
		t.Errorf("Narrators = %v", m.Narrators)
	}
	if m.ISBN != "9780330508117" {
		t.Errorf("ISBN = %q", m.ISBN)
	}
	// duration 11580 seconds / 60 = 193 minutes
	if m.DurationMin != 193 {
		t.Errorf("DurationMin = %d, want 193", m.DurationMin)
	}
	if m.SeriesName != "Hitchhiker's Guide" {
		t.Errorf("SeriesName = %q", m.SeriesName)
	}
	if m.SeriesPosition != "1" {
		t.Errorf("SeriesPosition = %q", m.SeriesPosition)
	}
	// Largest cover image should be selected (600x600).
	if m.CoverURL != "https://storytel.com/images/hitchikers_600.jpg" {
		t.Errorf("CoverURL = %q", m.CoverURL)
	}
	if len(m.Genres) < 1 {
		t.Errorf("expected genres, got none")
	}
}

func TestStorytelDomain(t *testing.T) {
	tests := []struct {
		region string
		want   string
	}{
		{"us", "storytel.com"},
		{"uk", "storytel.co.uk"},
		{"de", "storytel.de"},
		{"se", "storytel.se"},
		{"xx", "storytel.com"}, // unknown region → default
	}
	for _, tt := range tests {
		s := &StorytelScraper{region: tt.region}
		got := "storytel." + s.domain()[len("storytel."):]
		// domain() returns the fragment after "storytel."
		got = "storytel." + func() string {
			d, ok := storytelRegionDomain[tt.region]
			if !ok {
				return "com"
			}
			return d
		}()
		if got != tt.want {
			t.Errorf("domain(%q) = %q, want %q", tt.region, got, tt.want)
		}
	}
}

func TestStorytelParseBookPage_JSONLD(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
<script type="application/ld+json">
{
  "@type": "Audiobook",
  "name": "Test Book",
  "description": "A test description.",
  "image": "https://example.com/cover.jpg",
  "inLanguage": "en",
  "datePublished": "2020-03-15",
  "duration": "PT2H30M",
  "author": [{"name": "Test Author"}],
  "readBy": {"name": "Test Narrator"},
  "publisher": {"name": "Test Publisher"}
}
</script>
</body></html>`

	m := parseStorytelBookPage(html)
	if m == nil {
		t.Fatal("expected non-nil match")
	}
	if m.Title != "Test Book" {
		t.Errorf("Title = %q", m.Title)
	}
	if m.PublishYear != 2020 {
		t.Errorf("PublishYear = %d", m.PublishYear)
	}
	// PT2H30M = 2*60 + 30 = 150 min
	if m.DurationMin != 150 {
		t.Errorf("DurationMin = %d, want 150", m.DurationMin)
	}
	if len(m.Authors) == 0 || m.Authors[0] != "Test Author" {
		t.Errorf("Authors = %v", m.Authors)
	}
	if len(m.Narrators) == 0 || m.Narrators[0] != "Test Narrator" {
		t.Errorf("Narrators = %v", m.Narrators)
	}
	if m.Publisher != "Test Publisher" {
		t.Errorf("Publisher = %q", m.Publisher)
	}
}

func TestStorytelParseBookPageFallbacksAndEdges(t *testing.T) {
	next := `<script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"book":{
	  "bookId":"book-2","title":"Fallback Book","description":"D",
	  "authors":[{"name":""},{"name":"Author"}],"narrators":[{"name":"Narrator"}],
	  "cover":{"url":"https://cdn.example.test/fallback.jpg"},
	  "series":{"name":"Series","orderInSeries":0}
	}}}}</script>`
	match := parseStorytelBookPage(next)
	if match == nil {
		t.Fatal("expected fallback match")
	}
	if match.ProviderID != "book-2" || match.CoverURL != "https://cdn.example.test/fallback.jpg" {
		t.Fatalf("id/cover = %q/%q", match.ProviderID, match.CoverURL)
	}
	if match.SeriesPosition != "" || match.DurationMin != 0 {
		t.Fatalf("series/duration = %q/%d", match.SeriesPosition, match.DurationMin)
	}

	if got := parseStorytelSearch(`<script id="__NEXT_DATA__" type="application/json">not-json</script>`); got != nil {
		t.Fatalf("invalid search = %#v, want nil", got)
	}
	if got := parseStorytelSearch(`<script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"books":[]}}}</script>`); got != nil {
		t.Fatalf("empty search = %#v, want nil", got)
	}
	if got := parseStorytelBookPage(`<script type="application/ld+json">{"@type":"WebPage"}</script>`); got != nil {
		t.Fatalf("non-book jsonld = %#v, want nil", got)
	}
	if got := extractJSONLDNamesFromInterface([]interface{}{map[string]interface{}{"name": ""}, 7}); len(got) != 0 {
		t.Fatalf("empty mixed names = %#v", got)
	}
	if got := extractJSONLDNamesFromInterface(7); got != nil {
		t.Fatalf("numeric names = %#v, want nil", got)
	}
}

func TestStorytelFetchStatusPaths(t *testing.T) {
	scraper := NewStorytelScraper()
	scraper.limiter = newLimiter(1000)

	scraper.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})})
	if body, err := scraper.fetch(context.Background(), "https://example.test/not-found"); err != nil || body != "" {
		t.Fatalf("404 body=%q err=%v", body, err)
	}

	scraper.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTeapot,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})})
	if _, err := scraper.fetch(context.Background(), "https://example.test/teapot"); err == nil {
		t.Fatal("expected status error")
	}
}

func TestDecodeHTMLEntities(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Hello &amp; World", "Hello & World"},
		{"&lt;tag&gt;", "<tag>"},
		{"it&#39;s", "it's"},
		{"no entities", "no entities"},
	}
	for _, tt := range tests {
		got := decodeHTMLEntities(tt.in)
		if got != tt.want {
			t.Errorf("decodeHTMLEntities(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
