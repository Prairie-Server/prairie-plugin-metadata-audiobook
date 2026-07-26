package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/prairie-server/prairie-plugin-metadata-audiobook/metadata"
)

func TestAudnexusFetch(t *testing.T) {
	fixture, err := os.ReadFile("testdata/audnexus_book.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	defer srv.Close()

	client := NewAudnexusClient()
	client.baseURL = srv.URL

	m, err := client.Fetch(context.Background(), "B002V0QHBU")
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil match")
	}

	if m.Provider != "audnexus" {
		t.Errorf("Provider = %q, want %q", m.Provider, "audnexus")
	}
	if m.ASIN != "B002V0QHBU" {
		t.Errorf("ASIN = %q, want %q", m.ASIN, "B002V0QHBU")
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
	if m.DurationMin != 193 {
		t.Errorf("DurationMin = %d, want 193", m.DurationMin)
	}
	if m.PublishYear != 2005 {
		t.Errorf("PublishYear = %d, want 2005", m.PublishYear)
	}
	if m.SeriesName != "Hitchhiker's Guide to the Galaxy" {
		t.Errorf("SeriesName = %q", m.SeriesName)
	}
	if m.SeriesPosition != "1" {
		t.Errorf("SeriesPosition = %q, want %q", m.SeriesPosition, "1")
	}
	// Only "genre"-type entries should appear.
	if len(m.Genres) != 1 || m.Genres[0] != "Science Fiction" {
		t.Errorf("Genres = %v, want [Science Fiction]", m.Genres)
	}
	// HTML tags should be stripped.
	if m.Description == "" {
		t.Error("Description should not be empty after HTML strip")
	}
}

func TestAudnexusSearchByASIN(t *testing.T) {
	fixture, err := os.ReadFile("testdata/audnexus_book.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	defer srv.Close()

	client := NewAudnexusClient()
	client.baseURL = srv.URL

	q := metadata.SearchQuery{
		ProviderIDs: map[string]string{"asin": "B002V0QHBU"},
	}
	results, err := client.Search(context.Background(), q)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ASIN != "B002V0QHBU" {
		t.Errorf("ASIN = %q", results[0].ASIN)
	}
}

func TestAudnexusTitleSearch(t *testing.T) {
	// The title search endpoint returns an array of books.
	fixture, err := os.ReadFile("testdata/audnexus_book.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	arrayFixture := "[" + string(fixture) + "]"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(arrayFixture))
	}))
	defer srv.Close()

	client := NewAudnexusClient()
	client.baseURL = srv.URL

	q := metadata.SearchQuery{Title: "Hitchhiker"}
	results, err := client.Search(context.Background(), q)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Title != "The Hitchhiker's Guide to the Galaxy" {
		t.Errorf("Title = %q", results[0].Title)
	}
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"<p>Hello <b>World</b></p>", "Hello World"},
		{"No tags here", "No tags here"},
		{"&amp; &lt;tag&gt;", "& <tag>"},
	}
	for _, tt := range tests {
		got := stripHTML(tt.in)
		if got != tt.want {
			t.Errorf("stripHTML(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
