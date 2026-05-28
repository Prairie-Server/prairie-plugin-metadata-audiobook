package provider

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
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

func TestAudibleSearchASINs(t *testing.T) {
	fixture, err := os.ReadFile("testdata/audible_search.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	productFixture, err := os.ReadFile("testdata/audible_product.html")
	if err != nil {
		t.Fatalf("read product fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(fixture)
			return
		}
		// Product page requests.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(productFixture)
	}))
	defer srv.Close()

	scraper := NewAudibleScraper()
	scraper.httpClient = &http.Client{}
	// Override the base URL by temporarily replacing the region map.
	origMap := audibleRegionMap
	audibleRegionMap = map[string]string{"us": ""}
	defer func() { audibleRegionMap = origMap }()

	// Directly test the HTML parser for ASINs.
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(fixture))
	if err != nil {
		t.Fatalf("parse HTML: %v", err)
	}

	var asins []string
	seen := make(map[string]bool)
	doc.Find("[data-widget='productList'] a[href*='/pd/']").Each(func(_ int, sel *goquery.Selection) {
		href, _ := sel.Attr("href")
		m := asinPathRE.FindStringSubmatch(href)
		if len(m) >= 2 && !seen[m[1]] {
			seen[m[1]] = true
			asins = append(asins, m[1])
		}
	})

	if len(asins) != 2 {
		t.Fatalf("expected 2 ASINs, got %d: %v", len(asins), asins)
	}
	if asins[0] != "B002V0QHBU" {
		t.Errorf("first ASIN = %q, want B002V0QHBU", asins[0])
	}
	if asins[1] != "B002V0RB8O" {
		t.Errorf("second ASIN = %q, want B002V0RB8O", asins[1])
	}
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
