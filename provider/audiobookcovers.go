package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/time/rate"

	"github.com/Silo-Server/silo-plugin-audiobook-metadata/metadata"
)

const audiobookCoversBaseURL = "https://api.audiobookcovers.com"

// AudiobookCoversClient looks up cover art by ASIN.
type AudiobookCoversClient struct {
	httpClient *http.Client
	baseURL    string
	limiter    *rate.Limiter
}

func NewAudiobookCoversClient() *AudiobookCoversClient {
	return &AudiobookCoversClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    audiobookCoversBaseURL,
		limiter:    newLimiter(60),
	}
}

func (a *AudiobookCoversClient) Search(ctx context.Context, q metadata.SearchQuery) ([]metadata.Match, error) {
	asin := firstProviderID(q.ProviderIDs, "asin", "audiobookcovers")
	if asin == "" && isLikelyASIN(q.Title) {
		asin = q.Title
	}
	if !isLikelyASIN(asin) {
		return nil, nil
	}
	asin = normalizeASIN(asin)
	match, err := a.Fetch(ctx, asin)
	if err != nil || match == nil {
		return nil, err
	}
	return []metadata.Match{*match}, nil
}

func (a *AudiobookCoversClient) Fetch(ctx context.Context, asin string) (*metadata.Match, error) {
	if !isLikelyASIN(asin) {
		return nil, nil
	}
	asin = normalizeASIN(asin)
	if err := waitForLimiter(ctx, a.limiter); err != nil {
		return nil, err
	}
	reqURL := fmt.Sprintf("%s/cover/by_book/%s", a.baseURL, asin)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("audiobookcovers: create request: %w", err)
	}
	req.Header.Set("User-Agent", "silo-audiobook-metadata/audiobookcovers")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("audiobookcovers: request: %w", err)
	}
	body, err := readLimitedBody(resp, "audiobookcovers")
	if err != nil || body == "" {
		return nil, err
	}
	var parsed struct {
		CoverURL string `json:"cover_url"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return nil, fmt.Errorf("audiobookcovers: decode response: %w", err)
	}
	if parsed.CoverURL == "" {
		return nil, nil
	}
	return &metadata.Match{
		Provider:   "audiobookcovers",
		ProviderID: asin,
		ASIN:       asin,
		CoverURL:   parsed.CoverURL,
	}, nil
}
