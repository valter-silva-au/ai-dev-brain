package memory

import "context"

// EmbeddingProvider turns arbitrary text into a fixed-dimensional
// embedding vector. Implementations may call out to HTTP providers
// (OpenAI / Anthropic / Ollama) or compute locally (fake / in-tree).
//
// All implementations must guarantee: for a given text the returned
// vector's length equals Dimensions() and is stable within a single
// provider lifetime (caching is allowed; non-determinism is not).
type EmbeddingProvider interface {
	// Embed returns the embedding of text. Implementations should honour
	// ctx cancellation for network-bound providers.
	Embed(ctx context.Context, text string) ([]float32, error)

	// Dimensions reports the length of the vector Embed returns.
	Dimensions() int

	// Name is a short human-readable identifier used in logs and stored
	// in SQLite metadata so backends can detect model mismatches on
	// reopen (e.g. user switched from openai/text-embedding-3-small to
	// ollama/nomic-embed-text — different dims, store must refuse).
	Name() string
}
