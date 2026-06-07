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
