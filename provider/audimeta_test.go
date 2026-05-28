package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Silo-Server/silo-plugin-audiobook-metadata/metadata"
)

func TestAudiMetaFetch(t *testing.T) {
	fixture, err := os.ReadFile("testdata/audimeta_book.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	defer srv.Close()

	client := NewAudiMetaClient()
	client.baseURL = srv.URL

	m, err := client.Fetch(context.Background(), "B002V0QHBU")
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil match")
	}

	if m.Provider != "audimeta" {
		t.Errorf("Provider = %q, want %q", m.Provider, "audimeta")
	}
	if m.ASIN != "B002V0QHBU" {
		t.Errorf("ASIN = %q", m.ASIN)
	}
	if m.ISBN != "9780345391803" {
		t.Errorf("ISBN = %q", m.ISBN)
	}
	if m.SeriesName != "Hitchhiker's Guide to the Galaxy" {
		t.Errorf("SeriesName = %q", m.SeriesName)
	}
	if m.SeriesPosition != "1" {
		t.Errorf("SeriesPosition = %q", m.SeriesPosition)
	}
}

func TestAudiMetaSearch(t *testing.T) {
	fixture, err := os.ReadFile("testdata/audimeta_search.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	defer srv.Close()

	client := NewAudiMetaClient()
	client.baseURL = srv.URL

	q := metadata.SearchQuery{Title: "Hitchhiker"}
	results, err := client.Search(context.Background(), q)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	m := results[0]
	if m.Provider != "audimeta" {
		t.Errorf("Provider = %q", m.Provider)
	}
	if m.Title != "The Hitchhiker's Guide to the Galaxy" {
		t.Errorf("Title = %q", m.Title)
	}
	if len(m.Authors) == 0 || m.Authors[0] != "Douglas Adams" {
		t.Errorf("Authors = %v", m.Authors)
	}
	if m.SeriesName != "Hitchhiker's Guide to the Galaxy" {
		t.Errorf("SeriesName = %q", m.SeriesName)
	}
}
