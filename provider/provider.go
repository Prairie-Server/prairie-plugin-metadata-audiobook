package provider

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Silo-Server/silo-plugin-audiobook-metadata/metadata"
)

const (
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

	// Dispatch by provider hint.
	providerHint := q.ProviderIDs["provider"]

	switch providerHint {
	case "audnexus":
		asin := q.ProviderIDs["asin"]
		if asin == "" {
			asin = q.ProviderIDs["audnexus"]
		}
		if asin != "" {
			return p.Audnexus.Fetch(tctx, asin)
		}
	case "audimeta":
		asin := q.ProviderIDs["asin"]
		if asin == "" {
			asin = q.ProviderIDs["audimeta"]
		}
		if asin != "" {
			return p.AudiMeta.Fetch(tctx, asin)
		}
	case "itunes":
		return p.ITunes.Fetch(tctx, q.ProviderIDs["itunes"])
	case "audible":
		asin := q.ProviderIDs["asin"]
		if asin == "" {
			asin = q.ProviderIDs["audible"]
		}
		if asin != "" {
			return p.Audible.Fetch(tctx, asin)
		}
	case "storytel":
		id := q.ProviderIDs["storytel"]
		if id != "" {
			return p.Storytel.Fetch(tctx, id)
		}
	}

	// Fallback: if an ASIN is present, try Audnexus then AudiMeta.
	if asin := q.ProviderIDs["asin"]; asin != "" {
		if m, err := p.Audnexus.Fetch(tctx, asin); m != nil || err != nil {
			return m, err
		}
		return p.AudiMeta.Fetch(tctx, asin)
	}

	return nil, nil
}
