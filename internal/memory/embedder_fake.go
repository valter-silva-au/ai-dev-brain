package memory

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math"
)

// FakeEmbedder is a deterministic, no-network EmbeddingProvider for tests.
// It derives a fixed-size vector from a SHA-256 hash of the input, so the
// same text always produces the same vector. Vectors are L2-normalised
// (unit length) so cosine similarity behaves sanely.
//
// Not suitable for production — there is no semantic content. It exists
// so the Store's tests can run without network or external model
// dependencies.
type FakeEmbedder struct {
	Dim int
}

// NewFakeEmbedder constructs a FakeEmbedder with the given dimension.
// Panics if dim <= 0, because that's a test-time configuration error.
func NewFakeEmbedder(dim int) *FakeEmbedder {
	if dim <= 0 {
		panic("FakeEmbedder dim must be > 0")
	}
	return &FakeEmbedder{Dim: dim}
}

// Embed returns a deterministic unit vector derived from text.
func (f *FakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	// Seed a hash with the input, then fill the vector by extending
	// the hash deterministically (sha256 → 32 bytes → 8 float32s per
	// round; iterate until we have f.Dim components).
	vec := make([]float32, f.Dim)
	round := 0
	for filled := 0; filled < f.Dim; round++ {
		h := sha256.New()
		var roundBuf [8]byte
		binary.LittleEndian.PutUint64(roundBuf[:], uint64(round))
		h.Write([]byte(text))
		h.Write(roundBuf[:])
		digest := h.Sum(nil)
		for i := 0; i+4 <= len(digest) && filled < f.Dim; i += 4 {
			// Interpret 4 bytes as a uint32 then map to a float in
			// [-1, +1]. The range matters: cosine similarity is
			// symmetric on both halves of the unit sphere.
			u := binary.LittleEndian.Uint32(digest[i : i+4])
			vec[filled] = (float32(u)/float32(math.MaxUint32))*2 - 1
			filled++
		}
	}
	// L2-normalise so ||vec|| == 1. Cosine similarity then reduces to a
	// plain dot product, which keeps the arithmetic simple and stable.
	var sum float64
	for _, x := range vec {
		sum += float64(x) * float64(x)
	}
	norm := float32(math.Sqrt(sum))
	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}
	return vec, nil
}

// Dimensions returns the configured vector size.
func (f *FakeEmbedder) Dimensions() int { return f.Dim }

// Name identifies the provider in logs / stored metadata.
func (f *FakeEmbedder) Name() string { return "fake" }
