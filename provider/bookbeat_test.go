package provider

import "testing"

func TestBookBeatParseNextData(t *testing.T) {
	html := `<script id="__NEXT_DATA__" type="application/json">{
	  "props": {
	    "pageProps": {
	      "books": [{
	        "id": "midnight-library-98765",
	        "title": "The Midnight Library",
	        "description": "A library between lives.",
	        "authors": [{"name": "Matt Haig"}],
	        "narrators": [{"name": "Carey Mulligan"}],
	        "publisher": "Canongate Books",
	        "releaseDate": "2020-08-13",
	        "duration": 31740,
	        "language": "en",
	        "isbn": "9781786892737",
	        "coverUrl": "https://cdn.example.test/midnight.jpg",
	        "series": {"name": "Library", "orderInSeries": 1},
	        "categories": ["Fiction"]
	      }]
	    }
	  }
	}</script>`

	results := parseBookBeatSearch(html)
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	match := results[0]
	if match.Provider != "bookbeat" {
		t.Fatalf("Provider = %q, want bookbeat", match.Provider)
	}
	if match.Title != "The Midnight Library" {
		t.Fatalf("Title = %q", match.Title)
	}
	if len(match.Authors) != 1 || match.Authors[0] != "Matt Haig" {
		t.Fatalf("Authors = %v", match.Authors)
	}
	if len(match.Narrators) != 1 || match.Narrators[0] != "Carey Mulligan" {
		t.Fatalf("Narrators = %v", match.Narrators)
	}
	if match.DurationMin != 529 {
		t.Fatalf("DurationMin = %d, want 529", match.DurationMin)
	}
	if match.SeriesName != "Library" || match.SeriesPosition != "1" {
		t.Fatalf("series = %q/%q", match.SeriesName, match.SeriesPosition)
	}
}
