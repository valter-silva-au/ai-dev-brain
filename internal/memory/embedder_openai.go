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

// OpenAIEmbedder calls an OpenAI-compatible embeddings HTTP endpoint.
// The wire format is simple enough that this works for:
//
//   - OpenAI proper (endpoint: https://api.openai.com/v1/embeddings)
//   - Anthropic's embeddings API when exposed via an OpenAI-compatible shim
//   - Any other provider implementing the same request/response schema
//
// For Ollama, use OllamaEmbedder — its wire format is different.
type OpenAIEmbedder struct {
	Endpoint string // full URL including /v1/embeddings
	APIKey   string // Bearer token
	Model    string // e.g. "text-embedding-3-small"
	Dim      int    // expected dimensions; we validate the first response against this
	Timeout  time.Duration
	Client   *http.Client
}

type openAIEmbedRequest struct {
	Input string `json:"input"`
	Model string `json:"model"`
}

type openAIEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	// Error fields for when the HTTP status is non-2xx but we still get
	// a JSON body.
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Embed POSTs text to the configured endpoint and returns the embedding.
func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if e.Endpoint == "" {
		return nil, ErrInvalid{Reason: "OpenAIEmbedder.Endpoint is empty"}
	}

	body, err := json.Marshal(openAIEmbedRequest{Input: text, Model: e.Model})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.APIKey)
	}

	client := e.Client
	if client == nil {
		timeout := e.Timeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", e.Endpoint, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var parsed openAIEmbedResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode response (status %d): %w", resp.StatusCode, err)
	}

	if resp.StatusCode/100 != 2 {
		if parsed.Error != nil {
			return nil, fmt.Errorf("embed request failed (status %d): %s", resp.StatusCode, parsed.Error.Message)
		}
		return nil, fmt.Errorf("embed request failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	if len(parsed.Data) == 0 {
		return nil, fmt.Errorf("embed response missing data")
	}
	vec := parsed.Data[0].Embedding
	if e.Dim > 0 && len(vec) != e.Dim {
		return nil, fmt.Errorf("embed dim mismatch: got %d, configured %d", len(vec), e.Dim)
	}
	return vec, nil
}

// Dimensions reports the configured expected vector size. It may be 0
// before the first successful call if the caller didn't pre-declare it.
func (e *OpenAIEmbedder) Dimensions() int { return e.Dim }

// Name identifies the provider + model for stored metadata.
func (e *OpenAIEmbedder) Name() string {
	if e.Model == "" {
		return "openai"
	}
	return "openai/" + e.Model
}
