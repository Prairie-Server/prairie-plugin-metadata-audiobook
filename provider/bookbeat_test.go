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

func TestBookBeatParseNextDataCollectsNestedSiblingBooks(t *testing.T) {
	html := `<script id="__NEXT_DATA__" type="application/json">{
	  "props": {
	    "pageProps": {
	      "featured": {
	        "id": "first-book",
	        "title": "First Book",
	        "authors": [{"name": "Author One"}],
	        "related": [{
	          "id": "second-book",
	          "title": "Second Book",
	          "authors": [{"name": "Author Two"}]
	        }]
	      }
	    }
	  }
	}</script>`

	results := parseBookBeatSearch(html)
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Title != "First Book" || results[1].Title != "Second Book" {
		t.Fatalf("titles = %q, %q", results[0].Title, results[1].Title)
	}
}

func TestBookBeatParseNextDataPreservesDecimalSeriesPosition(t *testing.T) {
	html := `<script id="__NEXT_DATA__" type="application/json">{
	  "props": {
	    "pageProps": {
	      "book": {
	        "id": "middle-book",
	        "title": "Middle Book",
	        "authors": [{"name": "Author"}],
	        "series": {"name": "Series", "orderInSeries": 1.5}
	      }
	    }
	  }
	}</script>`

	results := parseBookBeatSearch(html)
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].SeriesPosition != "1.5" {
		t.Fatalf("SeriesPosition = %q, want 1.5", results[0].SeriesPosition)
	}
}

func TestBookBeatParseSearchLimitsAndInvalidData(t *testing.T) {
	if got := parseBookBeatSearch(`no next data`); got != nil {
		t.Fatalf("missing next data = %#v, want nil", got)
	}
	if got := parseBookBeatSearch(`<script id="__NEXT_DATA__" type="application/json">{bad</script>`); got != nil {
		t.Fatalf("invalid next data = %#v, want nil", got)
	}

	html := `<script id="__NEXT_DATA__" type="application/json">{"books":[`
	for i := 0; i < 25; i++ {
		if i > 0 {
			html += ","
		}
		html += `{"bookId":"id-` + string(rune('a'+i)) + `","title":"Title","authors":[]}`
	}
	html += `]}</script>`
	results := parseBookBeatSearch(html)
	if len(results) != 20 {
		t.Fatalf("len(results) = %d, want 20", len(results))
	}
	if isBookBeatBook(map[string]interface{}{"title": "Only Title"}) {
		t.Fatal("title-only object should not be treated as a book")
	}
}
