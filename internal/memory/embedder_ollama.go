package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaEmbedder calls a local Ollama server's /api/embeddings endpoint.
// Ollama's request/response schema differs from OpenAI's:
//
//	POST {endpoint}/api/embeddings
//	{"model": "nomic-embed-text", "prompt": "..."}
//
//	{"embedding": [0.1, 0.2, ...]}
//
// Typical endpoint for a local install: http://localhost:11434
type OllamaEmbedder struct {
	Endpoint string // base URL, e.g. http://localhost:11434
	Model    string // e.g. "nomic-embed-text"
	Dim      int    // expected dimensions; validated against the first response
	Timeout  time.Duration
	Client   *http.Client
}

type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaEmbedResponse struct {
	Embedding []float32 `json:"embedding"`
	Error     string    `json:"error,omitempty"`
}

// Embed POSTs text to Ollama and returns the embedding.
func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if e.Endpoint == "" {
		return nil, ErrInvalid{Reason: "OllamaEmbedder.Endpoint is empty"}
	}

	body, err := json.Marshal(ollamaEmbedRequest{Model: e.Model, Prompt: text})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := e.Endpoint + "/api/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := e.Client
	if client == nil {
		timeout := e.Timeout
		if timeout == 0 {
			timeout = 60 * time.Second // local model cold-start can be slow
		}
		client = &http.Client{Timeout: timeout}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var parsed ollamaEmbedResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode response (status %d): %w", resp.StatusCode, err)
	}

	if resp.StatusCode/100 != 2 {
		if parsed.Error != "" {
			return nil, fmt.Errorf("embed request failed (status %d): %s", resp.StatusCode, parsed.Error)
		}
		return nil, fmt.Errorf("embed request failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	if len(parsed.Embedding) == 0 {
		return nil, fmt.Errorf("embed response missing embedding")
	}
	if e.Dim > 0 && len(parsed.Embedding) != e.Dim {
		return nil, fmt.Errorf("embed dim mismatch: got %d, configured %d", len(parsed.Embedding), e.Dim)
	}
	return parsed.Embedding, nil
}

// Dimensions reports the configured expected vector size.
func (e *OllamaEmbedder) Dimensions() int { return e.Dim }

// Name identifies the provider + model for stored metadata.
func (e *OllamaEmbedder) Name() string {
	if e.Model == "" {
		return "ollama"
	}
	return "ollama/" + e.Model
}
