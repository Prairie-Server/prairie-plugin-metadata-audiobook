package main

import (
	"context"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-audiobook-metadata/provider"
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

func TestMetadataServerSearch_ReturnsEmpty(t *testing.T) {
	rs := &runtimeServer{provider: provider.NewProvider()}
	ms := &metadataServer{runtime: rs}

	resp, err := ms.Search(context.Background(), &pluginv1.SearchMetadataRequest{
		Query:    "The Name of the Wind",
		ItemType: "audiobook",
	})
	if err != nil {
		t.Fatalf("Search() returned error: %v", err)
	}
	if len(resp.GetResults()) != 0 {
		t.Fatalf("expected 0 results, got %d", len(resp.GetResults()))
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
