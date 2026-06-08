package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Silo-Server/silo-plugin-audiobook-metadata/metadata"
)

func TestAudiobookCoversSearchUsesASINProviderID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cover/by_book/B08G9PRS1K" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cover_url":"https://cdn.example.test/cover.jpg"}`))
	}))
	defer srv.Close()

	client := NewAudiobookCoversClient()
	client.baseURL = srv.URL
	client.httpClient = srv.Client()

	results, err := client.Search(context.Background(), metadata.SearchQuery{
		ProviderIDs: map[string]string{"asin": "b08g9prs1k"},
	})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].CoverURL != "https://cdn.example.test/cover.jpg" {
		t.Fatalf("CoverURL = %q", results[0].CoverURL)
	}
	if results[0].ASIN != "B08G9PRS1K" {
		t.Fatalf("ASIN = %q, want B08G9PRS1K", results[0].ASIN)
	}
}
