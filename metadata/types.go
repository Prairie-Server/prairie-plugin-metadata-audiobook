package metadata

// SearchQuery is the input every provider receives. Fields are
// optional unless documented; providers ignore what they cannot use.
type SearchQuery struct {
	Title       string
	Authors     []string
	Year        int
	ContentType string            // "audiobook" | "ebook" | "comic" | "manga"
	ProviderIDs map[string]string // e.g. {"asin": "B0...", "isbn": "978..."}
	Language    string
}

// Match is the unified shape every provider returns. Provider-specific
// fields stay empty when not applicable (e.g., Narrators is empty for
// ebooks). Description is plain text; HTML is stripped at the provider
// boundary.
type Match struct {
	Provider    string
	ProviderID  string
	Title       string
	Subtitle    string
	Authors     []string
	Narrators   []string // audiobook
	Description string
	Publisher   string
	PublishYear int
	ISBN        string
	ASIN        string
	Genres      []string
	CoverURL    string
	Language    string

	// Format-specific
	PageCount      int    // ebook
	DurationMin    int    // audiobook
	IssueNumber    string // comic
	VolumeNumber   string // comic/manga
	SeriesName     string
	SeriesPosition string
}
