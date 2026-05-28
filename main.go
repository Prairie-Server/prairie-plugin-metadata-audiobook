package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"

	"google.golang.org/protobuf/types/known/structpb"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	publicmanifest "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtime"
	"github.com/Silo-Server/silo-plugin-audiobook-metadata/metadata"
	"github.com/Silo-Server/silo-plugin-audiobook-metadata/provider"
)

// version is set at build time via -ldflags "-X main.version=...".
var version string

type runtimeServer struct {
	pluginv1.UnimplementedRuntimeServer

	manifest *pluginv1.PluginManifest
	provider *provider.Provider
}

type metadataServer struct {
	pluginv1.UnimplementedMetadataProviderServer
	runtime *runtimeServer
}

//go:embed manifest.json
var manifestJSON []byte

func (s *runtimeServer) GetManifest(context.Context, *pluginv1.GetManifestRequest) (*pluginv1.GetManifestResponse, error) {
	return &pluginv1.GetManifestResponse{Manifest: s.manifest}, nil
}

func (s *runtimeServer) Configure(_ context.Context, _ *pluginv1.ConfigureRequest) (*pluginv1.ConfigureResponse, error) {
	return &pluginv1.ConfigureResponse{}, nil
}

func (s *runtimeServer) providerForRequest() (*provider.Provider, error) {
	return s.provider, nil
}

func (s *metadataServer) Search(ctx context.Context, req *pluginv1.SearchMetadataRequest) (*pluginv1.SearchMetadataResponse, error) {
	p, err := s.runtime.providerForRequest()
	if err != nil {
		return nil, err
	}

	_, err = p.Search(ctx, metadata.SearchQuery{
		Title:       req.GetQuery(),
		Year:        int(req.GetYear()),
		ContentType: req.GetItemType(),
		ProviderIDs: stringMapFromStruct(req.GetProviderIds()),
		Language:    req.GetLanguage(),
	})
	if err != nil {
		return nil, err
	}

	// TODO(audiobook-metadata): map []metadata.Match to ProviderSearchResult records.
	return &pluginv1.SearchMetadataResponse{
		Results: []*pluginv1.ProviderSearchResult{},
	}, nil
}

func (s *metadataServer) GetMetadata(ctx context.Context, req *pluginv1.GetMetadataRequest) (*pluginv1.GetMetadataResponse, error) {
	p, err := s.runtime.providerForRequest()
	if err != nil {
		return nil, err
	}

	result, err := p.Fetch(ctx, metadata.SearchQuery{
		ProviderIDs: providerIDsFromProto(req.GetProviderIds(), "audiobook-metadata", req.GetProviderId()),
		ContentType: req.GetItemType(),
		Language:    req.GetLanguage(),
	})
	if err != nil || result == nil {
		return nil, err
	}

	// TODO(audiobook-metadata): map metadata.Match to MetadataItem.
	return &pluginv1.GetMetadataResponse{}, nil
}

func main() {
	manifest, err := loadManifest()
	if err != nil {
		panic(err)
	}

	rs := &runtimeServer{
		manifest: manifest,
		provider: provider.NewProvider(),
	}

	runtime.Serve(runtime.ServeConfig{
		Servers: runtime.CapabilityServers{
			Runtime:          rs,
			MetadataProvider: &metadataServer{runtime: rs},
		},
	})
}

func loadManifest() (*pluginv1.PluginManifest, error) {
	manifest, err := publicmanifest.Load(manifestJSON)
	if err != nil {
		return nil, fmt.Errorf("load embedded manifest: %w", err)
	}

	if version != "" {
		manifest.Version = version
	}

	executablePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable path: %w", err)
	}
	binaryData, err := os.ReadFile(executablePath)
	if err != nil {
		return nil, fmt.Errorf("read executable %q: %w", executablePath, err)
	}
	checksum := sha256.Sum256(binaryData)
	manifest.Checksum = hex.EncodeToString(checksum[:])

	return manifest, nil
}

func stringMapFromStruct(value *structpb.Struct) map[string]string {
	result := make(map[string]string)
	if value == nil {
		return result
	}
	for key, raw := range value.AsMap() {
		text, ok := raw.(string)
		if ok && text != "" {
			result[key] = text
		}
	}
	return result
}

func providerIDsFromProto(value *structpb.Struct, capabilityID string, fallbackID string) map[string]string {
	result := stringMapFromStruct(value)
	if fallbackID != "" && result[capabilityID] == "" {
		result[capabilityID] = fallbackID
	}
	return result
}
