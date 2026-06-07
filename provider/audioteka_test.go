package provider

import "testing"

func TestAudiotekaParseInitialState(t *testing.T) {
	html := `<script>window.__INITIAL_STATE__={
	  "catalog": {
	    "items": [{
	      "id": "wiedzmin-ostatnie-zyczenie",
	      "title": "Wiedźmin: Ostatnie życzenie",
	      "authors": [{"name": "Andrzej Sapkowski"}],
	      "narrators": [{"name": "Jacek Rozenek"}],
	      "publisher": "SuperNOWA",
	      "releaseDate": "2011-01-01",
	      "duration": 34020,
	      "language": "pl",
	      "isbn": "9788375170177",
	      "coverUrl": "https://cdn.example.test/wiedzmin.jpg",
	      "series": {"name": "Wiedźmin", "orderInSeries": 1},
	      "genres": ["Fantasy"]
	    }]
	  }
	};</script>`

	results := parseAudiotekaSearch(html)
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	match := results[0]
	if match.Provider != "audioteka" {
		t.Fatalf("Provider = %q, want audioteka", match.Provider)
	}
	if match.Title != "Wiedźmin: Ostatnie życzenie" {
		t.Fatalf("Title = %q", match.Title)
	}
	if len(match.Authors) != 1 || match.Authors[0] != "Andrzej Sapkowski" {
		t.Fatalf("Authors = %v", match.Authors)
	}
	if len(match.Narrators) != 1 || match.Narrators[0] != "Jacek Rozenek" {
		t.Fatalf("Narrators = %v", match.Narrators)
	}
	if match.DurationMin != 567 {
		t.Fatalf("DurationMin = %d, want 567", match.DurationMin)
	}
	if match.SeriesName != "Wiedźmin" || match.SeriesPosition != "1" {
		t.Fatalf("series = %q/%q", match.SeriesName, match.SeriesPosition)
	}
}

func TestAudiotekaParseInitialStateCollectsNestedSiblingBooks(t *testing.T) {
	html := `<script>window.__INITIAL_STATE__={
	  "catalog": {
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
	};</script>`

	results := parseAudiotekaSearch(html)
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Title != "First Book" || results[1].Title != "Second Book" {
		t.Fatalf("titles = %q, %q", results[0].Title, results[1].Title)
	}
}

func TestAudiotekaParseInitialStatePreservesDecimalSeriesPosition(t *testing.T) {
	html := `<script>window.__INITIAL_STATE__={
	  "catalog": {
	    "book": {
	      "id": "middle-book",
	      "title": "Middle Book",
	      "authors": [{"name": "Author"}],
	      "series": {"name": "Series", "orderInSeries": 1.5}
	    }
	  }
	};</script>`

	results := parseAudiotekaSearch(html)
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].SeriesPosition != "1.5" {
		t.Fatalf("SeriesPosition = %q, want 1.5", results[0].SeriesPosition)
	}
}
