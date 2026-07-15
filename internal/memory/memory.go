// Package memory provides adb's namespaced vector-memory substrate. Records
// are keyed by (namespace, key); content is embedded via a pluggable
// EmbeddingProvider; nearest-neighbour queries use an in-memory HNSW index
// layered over SQLite for persistence.
//
// Design sketched in .wiki/concepts/Vector Memory in adb.md on the consumer
// monorepo. Rationale in .wiki/decisions/0002-ruflo-dispatch-and-vector-
// memory-in-adb.md.
//
// Opt-in: default off. Enable via hooks.memory.enabled in .taskconfig.
// Go-native dependencies only (modernc.org/sqlite + github.com/coder/hnsw
// v0.2.0) so the adb binary stays cgo-free and single-file on all three
// supported OSes.
package memory

import (
	"context"
	"fmt"
)

// Store is the persistence + retrieval contract for adb's vector memory.
// All operations are namespace-scoped: a Search in namespace "tickets/X"
// never returns hits from "sessions/Y" (prevents cross-contamination
// between different kinds of memory).
type Store interface {
	// Upsert inserts or replaces a record at (ns, key) with content.
	// Meta is a free-form string->string map persisted alongside.
	Upsert(ctx context.Context, ns, key, content string, meta map[string]string) error

	// Search returns up to k nearest neighbours for query within ns,
	// ranked by descending similarity (higher Score is more similar).
	// Returns nil slice (not error) when the namespace is empty.
	Search(ctx context.Context, ns, query string, k int) ([]Hit, error)

	// Delete removes the record at (ns, key). Missing records are a
	// no-op, not an error.
	Delete(ctx context.Context, ns, key string) error

	// ListNamespaces returns all namespaces that currently hold records.
	ListNamespaces(ctx context.Context) ([]string, error)

	// Close releases underlying resources (database handles, etc.).
	Close() error
}

// Hit is one result of a Search call.
type Hit struct {
	Namespace string
	Key       string
	// Score is in [0, 1] where 1 is most similar. Implementations using
	// cosine distance surface (1 - distance); higher is better.
	Score   float32
	Content string
	Meta    map[string]string
}

// Entry is the stored form of a record (used by export / import paths).
type Entry struct {
	Namespace string            `json:"namespace"`
	Key       string            `json:"key"`
	Content   string            `json:"content"`
	Meta      map[string]string `json:"meta,omitempty"`
}

// ErrInvalid indicates a caller-side argument violation (empty namespace,
// empty key, etc.). Distinct from storage / embedding errors so callers
// can differentiate "I misused the API" from "the backend failed".
type ErrInvalid struct{ Reason string }

func (e ErrInvalid) Error() string { return fmt.Sprintf("invalid argument: %s", e.Reason) }

// validateUpsert is a small helper reused across Store implementations.
// Kept in the package root so tests can assert the same contract
// regardless of backend.
func validateUpsert(ns, key string) error {
	if ns == "" {
		return ErrInvalid{Reason: "namespace must not be empty"}
	}
	if key == "" {
		return ErrInvalid{Reason: "key must not be empty"}
	}
	return nil
}
