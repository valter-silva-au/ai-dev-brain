package memory

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// EmbedderConfig is the provider-agnostic description of which embedding
// provider to build. It is the single place that maps provider→embedder, shared
// by the `adb memory` CLI, the memory hook indexer, and the MCP
// search_knowledge tool so the three cannot drift.
type EmbedderConfig struct {
	Provider string // "" | "fake" | "openai" | "ollama"
	Model    string
	Endpoint string
	APIKey   string // supports "$ENV_VAR" interpolation
	Dim      int    // <= 0 defaults to 64 (the fake provider's sensible default)
}

// NewEmbedder builds an EmbeddingProvider from cfg. The fake provider is the
// default (no network, deterministic); openai/ollama get a sensible default
// endpoint and HTTP client. An unknown provider is an error.
func NewEmbedder(cfg EmbedderConfig) (EmbeddingProvider, error) {
	dim := cfg.Dim
	if dim <= 0 {
		dim = 64
	}
	apiKey := cfg.APIKey
	if strings.HasPrefix(apiKey, "$") {
		apiKey = os.Getenv(strings.TrimPrefix(apiKey, "$"))
	}
	switch strings.ToLower(cfg.Provider) {
	case "", "fake":
		return NewFakeEmbedder(dim), nil
	case "openai":
		endpoint := cfg.Endpoint
		if endpoint == "" {
			endpoint = "https://api.openai.com/v1/embeddings"
		}
		return &OpenAIEmbedder{
			Endpoint: endpoint,
			APIKey:   apiKey,
			Model:    cfg.Model,
			Dim:      dim,
			Client:   &http.Client{Timeout: 30 * time.Second},
		}, nil
	case "ollama":
		endpoint := cfg.Endpoint
		if endpoint == "" {
			endpoint = "http://localhost:11434"
		}
		return &OllamaEmbedder{
			Endpoint: endpoint,
			Model:    cfg.Model,
			Dim:      dim,
			Client:   &http.Client{Timeout: 60 * time.Second},
		}, nil
	default:
		return nil, fmt.Errorf("unknown provider %q (valid: fake, openai, ollama)", cfg.Provider)
	}
}
