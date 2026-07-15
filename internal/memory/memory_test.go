package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"pgregory.net/rapid"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "memory.sqlite")
	s, err := OpenSQLiteStore(context.Background(), dbPath, NewFakeEmbedder(64))
	if err != nil {
		t.Fatalf("OpenSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestSQLiteStore_UpsertSearch_SelfQueryTopHit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.Upsert(ctx, "tickets/T-1", "note", "the quick brown fox jumps over the lazy dog", nil); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	hits, err := s.Search(ctx, "tickets/T-1", "the quick brown fox jumps over the lazy dog", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("Search returned 0 hits for exact-match query")
	}
	if hits[0].Key != "note" {
		t.Errorf("Search top hit key = %q, want %q", hits[0].Key, "note")
	}
	if hits[0].Score < 0.99 {
		t.Errorf("exact-match score = %v, expected ~1.0", hits[0].Score)
	}
	if hits[0].Namespace != "tickets/T-1" {
		t.Errorf("Search returned cross-namespace hit: %q", hits[0].Namespace)
	}
}

func TestSQLiteStore_Search_NamespaceIsolation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.Upsert(ctx, "tickets/A", "a1", "alpha beta gamma", nil); err != nil {
		t.Fatalf("Upsert A: %v", err)
	}
	if err := s.Upsert(ctx, "tickets/B", "b1", "alpha beta gamma", nil); err != nil {
		t.Fatalf("Upsert B: %v", err)
	}

	hits, err := s.Search(ctx, "tickets/A", "alpha", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit in namespace A, got %d (%v)", len(hits), hits)
	}
	if hits[0].Namespace != "tickets/A" {
		t.Errorf("namespace leak: got %q, want tickets/A", hits[0].Namespace)
	}
	if hits[0].Key != "a1" {
		t.Errorf("wrong key: got %q, want a1", hits[0].Key)
	}
}

func TestSQLiteStore_Upsert_Update(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.Upsert(ctx, "ns", "k", "initial content here", nil); err != nil {
		t.Fatalf("Upsert initial: %v", err)
	}
	if err := s.Upsert(ctx, "ns", "k", "updated content replaces it", map[string]string{"rev": "2"}); err != nil {
		t.Fatalf("Upsert update: %v", err)
	}

	hits, err := s.Search(ctx, "ns", "updated content replaces it", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("Search returned 0 hits after update")
	}
	if hits[0].Content != "updated content replaces it" {
		t.Errorf("content not updated: %q", hits[0].Content)
	}
	if hits[0].Meta["rev"] != "2" {
		t.Errorf("meta not updated: %v", hits[0].Meta)
	}
}

func TestSQLiteStore_Delete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.Upsert(ctx, "ns", "k", "content to delete", nil); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := s.Delete(ctx, "ns", "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	hits, err := s.Search(ctx, "ns", "content to delete", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("expected 0 hits after delete, got %d: %v", len(hits), hits)
	}
	// Deleting a missing record is a no-op.
	if err := s.Delete(ctx, "ns", "k"); err != nil {
		t.Errorf("Delete of missing record should be no-op, got %v", err)
	}
}

func TestSQLiteStore_ListNamespaces(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if got, err := s.ListNamespaces(ctx); err != nil {
		t.Fatalf("empty list: %v", err)
	} else if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}

	for _, ns := range []string{"alpha", "beta", "gamma"} {
		if err := s.Upsert(ctx, ns, "k", "content "+ns, nil); err != nil {
			t.Fatalf("Upsert %s: %v", ns, err)
		}
	}
	got, err := s.ListNamespaces(ctx)
	if err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}
	sort.Strings(got)
	want := []string{"alpha", "beta", "gamma"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("namespaces[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSQLiteStore_PersistsAcrossReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "persist.sqlite")
	ctx := context.Background()
	emb := NewFakeEmbedder(64)

	// Open, write, close.
	s1, err := OpenSQLiteStore(ctx, dbPath, emb)
	if err != nil {
		t.Fatalf("open 1: %v", err)
	}
	if err := s1.Upsert(ctx, "ns", "persist-me", "this content must survive a close", nil); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close 1: %v", err)
	}

	// Reopen, query.
	s2, err := OpenSQLiteStore(ctx, dbPath, emb)
	if err != nil {
		t.Fatalf("open 2: %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })

	hits, err := s2.Search(ctx, "ns", "this content must survive a close", 1)
	if err != nil {
		t.Fatalf("Search after reopen: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("entry did not survive reopen")
	}
	if hits[0].Key != "persist-me" {
		t.Errorf("wrong key after reopen: %q", hits[0].Key)
	}
}

func TestSQLiteStore_EmbedderMismatch_Refuses(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mismatch.sqlite")
	ctx := context.Background()

	// First open with fake dim=64.
	s1, err := OpenSQLiteStore(ctx, dbPath, NewFakeEmbedder(64))
	if err != nil {
		t.Fatalf("open 1: %v", err)
	}
	_ = s1.Close()

	// Reopen with fake dim=128 — must refuse.
	_, err = OpenSQLiteStore(ctx, dbPath, NewFakeEmbedder(128))
	if err == nil {
		t.Fatal("expected dim-mismatch error, got nil")
	}
}

func TestSQLiteStore_ValidatesArgs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cases := []struct {
		name string
		op   func() error
	}{
		{"Upsert empty ns", func() error { return s.Upsert(ctx, "", "k", "c", nil) }},
		{"Upsert empty key", func() error { return s.Upsert(ctx, "ns", "", "c", nil) }},
		{"Upsert RS in ns", func() error { return s.Upsert(ctx, "a\x1eb", "k", "c", nil) }},
		{"Upsert RS in key", func() error { return s.Upsert(ctx, "ns", "a\x1eb", "c", nil) }},
		{"Delete empty ns", func() error { return s.Delete(ctx, "", "k") }},
		{"Delete empty key", func() error { return s.Delete(ctx, "ns", "") }},
		{"Search empty ns", func() error { _, e := s.Search(ctx, "", "q", 1); return e }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.op(); err == nil {
				t.Errorf("expected validation error, got nil")
			}
		})
	}
}

func TestFakeEmbedder_Deterministic(t *testing.T) {
	e := NewFakeEmbedder(32)
	v1, err := e.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("embed 1: %v", err)
	}
	v2, err := e.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("embed 2: %v", err)
	}
	if len(v1) != 32 || len(v2) != 32 {
		t.Fatalf("dim = %d / %d, want 32", len(v1), len(v2))
	}
	for i := range v1 {
		if v1[i] != v2[i] {
			t.Errorf("non-deterministic at index %d: %v vs %v", i, v1[i], v2[i])
		}
	}
	// Different input → different output.
	v3, _ := e.Embed(context.Background(), "different input")
	same := true
	for i := range v1 {
		if v1[i] != v3[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("FakeEmbedder returned identical vectors for different inputs")
	}
}

func TestFakeEmbedder_UnitNormalised(t *testing.T) {
	e := NewFakeEmbedder(128)
	v, _ := e.Embed(context.Background(), "normalise me")
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	// Allow some float32 rounding slack.
	if sum < 0.99 || sum > 1.01 {
		t.Errorf("expected unit vector, got |v|^2 = %v", sum)
	}
}

func TestCosineSimilarity(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	if got := cosineSimilarity(a, b); got < 0.999 {
		t.Errorf("identical vectors: got %v, want ~1.0", got)
	}
	c := []float32{0, 1, 0}
	if got := cosineSimilarity(a, c); got < -0.001 || got > 0.001 {
		t.Errorf("orthogonal vectors: got %v, want ~0.0", got)
	}
	d := []float32{-1, 0, 0}
	if got := cosineSimilarity(a, d); got > -0.999 {
		t.Errorf("opposite vectors: got %v, want ~-1.0", got)
	}
	// Length mismatch → 0.
	if got := cosineSimilarity(a, []float32{1, 0}); got != 0 {
		t.Errorf("length mismatch: got %v, want 0", got)
	}
}

func TestEncodeDecodeVector_Roundtrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 256).Draw(t, "n")
		v := make([]float32, n)
		for i := range v {
			v[i] = rapid.Float32().Draw(t, fmt.Sprintf("v[%d]", i))
		}
		encoded := encodeVector(v)
		decoded, err := decodeVector(encoded)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(decoded) != len(v) {
			t.Fatalf("length: got %d, want %d", len(decoded), len(v))
		}
		for i := range v {
			if decoded[i] != v[i] {
				t.Errorf("index %d: got %v, want %v", i, decoded[i], v[i])
			}
		}
	})
}

func TestSQLiteStore_ManyRecords_TopHit(t *testing.T) {
	// Rapid-powered: stores N distinct contents, then queries by one of
	// them and asserts that content is the top hit.
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(5, 50).Draw(t, "n")
		tmp, err := os.MkdirTemp("", "rapid-mem-*")
		if err != nil {
			t.Fatalf("mktemp: %v", err)
		}
		defer os.RemoveAll(tmp)
		dbPath := filepath.Join(tmp, "rapid.sqlite")
		ctx := context.Background()
		s, err := OpenSQLiteStore(ctx, dbPath, NewFakeEmbedder(32))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer s.Close()

		contents := make([]string, n)
		for i := 0; i < n; i++ {
			// Unique content per entry so FakeEmbedder produces distinct
			// vectors.
			contents[i] = fmt.Sprintf("rapid test entry number %d with unique salt %d", i, i*7919)
			if err := s.Upsert(ctx, "ns", fmt.Sprintf("k%d", i), contents[i], nil); err != nil {
				t.Fatalf("Upsert %d: %v", i, err)
			}
		}

		// Pick one at random to query.
		idx := rapid.IntRange(0, n-1).Draw(t, "idx")
		hits, err := s.Search(ctx, "ns", contents[idx], 1)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(hits) == 0 {
			t.Fatalf("Search returned 0 hits")
		}
		if hits[0].Key != fmt.Sprintf("k%d", idx) {
			t.Errorf("top hit key = %s, want k%d (score=%v)", hits[0].Key, idx, hits[0].Score)
		}
	})
}
