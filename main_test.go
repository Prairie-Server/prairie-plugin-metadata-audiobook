package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pluginv1 "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginproto/prairie/plugin/v1"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/prairie-server/prairie-plugin-metadata-audiobook/metadata"
	"github.com/prairie-server/prairie-plugin-metadata-audiobook/provider"
)

func TestRuntimeServerConfigure_NoOp(t *testing.T) {
	server := &runtimeServer{provider: provider.NewProvider()}

	_, err := server.Configure(context.Background(), &pluginv1.ConfigureRequest{})
	if err != nil {
		t.Fatalf("Configure() returned error: %v", err)
	}

	p, err := server.providerForRequest()
	if err != nil {
		t.Fatalf("providerForRequest() returned error: %v", err)
	}
	if p == nil {
		t.Fatal("expected provider to be available")
	}
}

func TestMetadataServerGetMetadata_ReturnsNilForUnknown(t *testing.T) {
	rs := &runtimeServer{provider: provider.NewProvider()}
	ms := &metadataServer{runtime: rs}

	resp, err := ms.GetMetadata(context.Background(), &pluginv1.GetMetadataRequest{
		ProviderId: "unknown-id",
		ItemType:   "audiobook",
	})
	if err != nil {
		t.Fatalf("GetMetadata() returned error: %v", err)
	}
	if resp.GetItem() != nil {
		t.Fatalf("expected nil item, got %v", resp.GetItem())
	}
}

func TestProviderSearchResultFromMatchMapsProviderIDs(t *testing.T) {
	result, err := providerSearchResultFromMatch(metadata.Match{
		Provider:    "audible",
		ProviderID:  "B012345678",
		Title:       "The Name of the Wind",
		PublishYear: 2007,
		Description: "A book summary",
		ASIN:        "B012345678",
		CoverURL:    "https://example.test/cover.jpg",
	}, "audiobook")
	if err != nil {
		t.Fatalf("providerSearchResultFromMatch() error = %v", err)
	}
	if result.GetProviderId() != "B012345678" {
		t.Fatalf("ProviderId = %q, want B012345678", result.GetProviderId())
	}
	if result.GetTitle() != "The Name of the Wind" {
		t.Fatalf("Title = %q", result.GetTitle())
	}

	ids := result.GetProviderIds().AsMap()
	if _, ok := ids["provider"]; ok {
		t.Fatalf("provider ids should not include synthetic provider hint: %#v", ids)
	}
	if ids["audible"] != "B012345678" {
		t.Fatalf("provider ids = %#v", ids)
	}
	if ids[capabilityID] != "B012345678" || ids["asin"] != "B012345678" {
		t.Fatalf("capability/asin ids = %#v", ids)
	}
}

func TestMetadataItemFromMatchMapsAudiobookFields(t *testing.T) {
	item, err := metadataItemFromMatch(metadata.Match{
		Provider:       "audible",
		ProviderID:     "B012345678",
		Title:          "The Name of the Wind",
		Authors:        []string{"Patrick Rothfuss"},
		Narrators:      []string{"Nick Podehl"},
		Description:    "A book summary",
		Publisher:      "DAW",
		PublishYear:    2007,
		ASIN:           "B012345678",
		Genres:         []string{"Fantasy"},
		CoverURL:       "https://example.test/cover.jpg",
		DurationMin:    1650,
		SeriesName:     "The Kingkiller Chronicle",
		SeriesPosition: "1",
	}, "audiobook")
	if err != nil {
		t.Fatalf("metadataItemFromMatch() error = %v", err)
	}
	if item.GetProviderId() != "B012345678" || item.GetItemType() != "audiobook" {
		t.Fatalf("item ids/type = %q/%q", item.GetProviderId(), item.GetItemType())
	}
	if item.GetPosterPath() != "https://example.test/cover.jpg" {
		t.Fatalf("PosterPath = %q", item.GetPosterPath())
	}
	if item.GetRuntime() != 1650 {
		t.Fatalf("Runtime = %d, want 1650", item.GetRuntime())
	}
	if len(item.GetStudios()) != 1 || item.GetStudios()[0] != "DAW" {
		t.Fatalf("Studios = %#v", item.GetStudios())
	}
	if len(item.GetPeople()) != 2 {
		t.Fatalf("people = %#v", item.GetPeople())
	}
	if item.GetPeople()[0].GetKind() != "author" || item.GetPeople()[1].GetKind() != "narrator" {
		t.Fatalf("people kinds = %#v", item.GetPeople())
	}
	if item.GetMetadata().AsMap()["series_name"] != "The Kingkiller Chronicle" {
		t.Fatalf("metadata = %#v", item.GetMetadata().AsMap())
	}
}

func TestLoadManifestAndGetManifest(t *testing.T) {
	original := version
	version = "3.3.3-test"
	t.Cleanup(func() { version = original })
	manifest, err := loadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.GetVersion() != "3.3.3-test" || len(manifest.GetChecksum()) != 64 {
		t.Fatalf("manifest=%q checksum=%q", manifest.GetVersion(), manifest.GetChecksum())
	}
	rs := &runtimeServer{manifest: manifest, provider: provider.NewProvider()}
	resp, err := rs.GetManifest(context.Background(), &pluginv1.GetManifestRequest{})
	if err != nil || resp.GetManifest() != manifest {
		t.Fatalf("GetManifest err=%v", err)
	}
}

func TestMetadataServerSearch(t *testing.T) {
	bookHTML := `<script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"books":[{
	  "id":"midnight-library","title":"The Midnight Library","authors":[{"name":"Matt Haig"}],
	  "coverUrl":"https://cdn.example.test/c.jpg","duration":100,"releaseDate":"2020-01-01"
	}]}}}</script>`
	scrape := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(bookHTML))
	}))
	t.Cleanup(scrape.Close)
	emptyJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"resultCount": 0, "results": []any{}, "books": []any{}})
	}))
	t.Cleanup(emptyJSON.Close)
	quiet := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header), Request: req}, nil
	})}

	p := provider.NewProvider()
	p.Audnexus.SetBaseURL(emptyJSON.URL)
	p.AudiMeta.SetBaseURL(emptyJSON.URL)
	p.ITunes.SetBaseURL(emptyJSON.URL)
	p.AudiobookCovers.SetBaseURL(emptyJSON.URL)
	p.BookBeat.SetBaseURL(scrape.URL)
	p.Audioteka.SetBaseURL(scrape.URL)
	p.Audible.SetHTTPClient(quiet)
	p.Storytel.SetHTTPClient(quiet)

	ms := &metadataServer{runtime: &runtimeServer{provider: p}}
	resp, err := ms.Search(context.Background(), &pluginv1.SearchMetadataRequest{Query: "Midnight", ItemType: "audiobook"})
	if err != nil {
		t.Fatalf("Search err=%v", err)
	}
	if len(resp.GetResults()) == 0 {
		t.Fatal("expected search results")
	}
}

func TestMetadataServerGetMetadataSuccess(t *testing.T) {
	bookHTML := `<script type="application/ld+json">{"@type":"Audiobook","name":"JSONLD Book","author":{"name":"A"},"image":"https://cdn.example.test/j.jpg","description":"D"}</script>`
	scrape := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(bookHTML))
	}))
	t.Cleanup(scrape.Close)

	p := provider.NewProvider()
	p.BookBeat.SetBaseURL(scrape.URL)
	ms := &metadataServer{runtime: &runtimeServer{provider: p}}
	meta, err := ms.GetMetadata(context.Background(), &pluginv1.GetMetadataRequest{
		ProviderId: "midnight-library", ItemType: "audiobook",
		ProviderIds: mustStruct(t, map[string]any{"bookbeat": "midnight-library"}),
	})
	if err != nil || meta.GetItem() == nil {
		t.Fatalf("GetMetadata %#v err=%v", meta, err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func mustStruct(t *testing.T, value map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(value)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMainHelpersAudiobook(t *testing.T) {
	s, err := stringStruct(map[string]string{"a": "1", "b": ""})
	if err != nil || s == nil {
		t.Fatal(s, err)
	}
	if s, err := stringStruct(nil); err != nil || s != nil {
		t.Fatal(s, err)
	}
	if stringMapFromStruct(nil) == nil {
		t.Fatal("nil map")
	}
	st, _ := structpb.NewStruct(map[string]any{"audible": "B012345678", "x": 1})
	m := stringMapFromStruct(st)
	if m["audible"] != "B012345678" {
		t.Fatal(m)
	}
	ids := providerIDsFromProto(st, capabilityID, "fallback")
	if ids[capabilityID] != "fallback" {
		t.Fatalf("capability fallback = %q, want fallback", ids[capabilityID])
	}
	if ids["audible"] != "B012345678" {
		t.Fatalf("audible id = %q, want B012345678", ids["audible"])
	}
	if primaryProviderID(metadata.Match{ProviderID: "id1", ASIN: "B0"}) != "id1" {
		t.Fatal("primary")
	}
	if primaryProviderID(metadata.Match{ASIN: "B012345678"}) != "B012345678" {
		t.Fatal("asin primary")
	}
	if publisherStudio("") != nil {
		t.Fatal("empty studio")
	}
	item, err := metadataItemFromMatch(metadata.Match{
		Provider: "itunes", ProviderID: "99", Title: "T", Authors: []string{"A"}, Narrators: []string{"N"},
		Genres: []string{" G ", ""}, Publisher: "P", DurationMin: 10, CoverURL: "https://x",
		SeriesName: "S", SeriesPosition: "2", Language: "en", ISBN: "978",
	}, "audiobook")
	if err != nil || item == nil {
		t.Fatal(err)
	}
	result, err := providerSearchResultFromMatch(metadata.Match{Provider: "storytel", ProviderID: "s1", Title: "T"}, "audiobook")
	if err != nil || result == nil {
		t.Fatal(err)
	}
}

func TestPrimaryProviderIDAndPeopleEdges(t *testing.T) {
	if primaryProviderID(metadata.Match{}) != "" {
		t.Fatal("empty")
	}
	people := peopleFromMatch(metadata.Match{Authors: []string{"", "A"}, Narrators: []string{"N", ""}})
	if len(people) != 2 {
		t.Fatalf("%#v", people)
	}
	item, err := metadataItemFromMatch(metadata.Match{Provider: "x", ProviderID: "1", Title: "T"}, "audiobook")
	if err != nil || item == nil {
		t.Fatal(err)
	}
}
