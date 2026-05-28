package provider

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Silo-Server/silo-plugin-audiobook-metadata/metadata"
)

const (
	capabilityProviderID = "audiobook-metadata"

	// providerTimeout is the per-provider deadline for Search/Fetch calls.
	providerTimeout = 10 * time.Second

	// searchWorkers is the maximum number of providers queried in parallel.
	searchWorkers = 3
)

// searchProvider is the minimal interface each backend must satisfy.
type searchProvider interface {
	Search(ctx context.Context, q metadata.SearchQuery) ([]metadata.Match, error)
}

// Provider is the coordinator for all audiobook metadata sources.
type Provider struct {
	Audnexus *AudnexusClient
	AudiMeta *AudiMetaClient
	ITunes   *ITunesClient
	Audible  *AudibleScraper
	Storytel *StorytelScraper
}

// NewProvider creates a Provider with all source clients initialized.
func NewProvider() *Provider {
	return &Provider{
		Audnexus: NewAudnexusClient(),
		AudiMeta: NewAudiMetaClient(),
		ITunes:   NewITunesClient(),
		Audible:  NewAudibleScraper(),
		Storytel: NewStorytelScraper(),
	}
}

// Search queries all registered providers in parallel (up to searchWorkers
// concurrent goroutines) and returns the merged results.
// Errors from individual providers are logged but are not fatal.
func (p *Provider) Search(ctx context.Context, q metadata.SearchQuery) ([]metadata.Match, error) {
	type task struct {
		name string
		fn   searchProvider
	}

	tasks := []task{
		{"audnexus", p.Audnexus},
		{"audimeta", p.AudiMeta},
		{"itunes", p.ITunes},
		{"audible", p.Audible},
		{"storytel", p.Storytel},
	}

	type result struct {
		matches []metadata.Match
		err     error
	}

	ch := make(chan result, len(tasks))
	sem := make(chan struct{}, searchWorkers)

	var wg sync.WaitGroup
	for _, t := range tasks {
		wg.Add(1)
		go func(name string, sp searchProvider) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			tctx, cancel := context.WithTimeout(ctx, providerTimeout)
			defer cancel()

			matches, err := sp.Search(tctx, q)
			if err != nil {
				log.Printf("audiobook-metadata: provider %s search error: %v", name, err)
				ch <- result{err: err}
				return
			}
			ch <- result{matches: matches}
		}(t.name, t.fn)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var all []metadata.Match
	for r := range ch {
		all = append(all, r.matches...)
	}

	return all, nil
}

// Fetch retrieves full metadata for a specific item by providerID and externalID.
// providerID selects which backend to use (e.g. "audnexus", "audimeta", "itunes",
// "audible", "storytel"). externalID is the backend-specific item identifier.
func (p *Provider) Fetch(ctx context.Context, q metadata.SearchQuery) (*metadata.Match, error) {
	tctx, cancel := context.WithTimeout(ctx, providerTimeout)
	defer cancel()

	// Dispatch by explicit legacy hint or by the real provider-specific IDs
	// stored in ProviderIDs.
	providerHint := providerHintFromIDs(q.ProviderIDs)

	switch providerHint {
	case "audnexus":
		asin := firstProviderID(q.ProviderIDs, "asin", "audnexus", capabilityProviderID)
		if asin != "" {
			return p.Audnexus.Fetch(tctx, asin)
		}
	case "audimeta":
		asin := firstProviderID(q.ProviderIDs, "asin", "audimeta", capabilityProviderID)
		if asin != "" {
			return p.AudiMeta.Fetch(tctx, asin)
		}
	case "itunes":
		if id := firstProviderID(q.ProviderIDs, "itunes", capabilityProviderID); id != "" {
			return p.ITunes.Fetch(tctx, id)
		}
	case "audible":
		asin := firstProviderID(q.ProviderIDs, "asin", "audible", capabilityProviderID)
		if asin != "" {
			return p.Audible.Fetch(tctx, asin)
		}
	case "storytel":
		if id := firstProviderID(q.ProviderIDs, "storytel", capabilityProviderID); id != "" {
			return p.Storytel.Fetch(tctx, id)
		}
	}

	// Fallback: if an ASIN is present, try Audnexus then AudiMeta. Only
	// treat the capability-level ID as an ASIN when it has ASIN shape, since
	// non-ASIN providers such as iTunes also use that field as a fallback ID.
	asin := firstProviderID(q.ProviderIDs, "asin", "audnexus", "audimeta", "audible")
	if asin == "" {
		if id := firstProviderID(q.ProviderIDs, capabilityProviderID); isLikelyASIN(id) {
			asin = id
		}
	}
	if asin != "" {
		if m, err := p.Audnexus.Fetch(tctx, asin); m != nil || err != nil {
			return m, err
		}
		return p.AudiMeta.Fetch(tctx, asin)
	}

	return nil, nil
}

func providerHintFromIDs(ids map[string]string) string {
	if hint := strings.TrimSpace(ids["provider"]); hint != "" {
		return hint
	}
	for _, provider := range []string{"audnexus", "audimeta", "itunes", "audible", "storytel"} {
		if strings.TrimSpace(ids[provider]) != "" {
			return provider
		}
	}
	return ""
}

func isLikelyASIN(value string) bool {
	if len(value) != 10 {
		return false
	}
	for _, ch := range value {
		if (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			continue
		}
		return false
	}
	return true
}

func firstProviderID(ids map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(ids[key]); value != "" {
			return value
		}
	}
	return ""
}
