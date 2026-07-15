package memory

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coder/hnsw"
	_ "modernc.org/sqlite"
)

const (
	compositeKeySep = "\x1e" // ASCII record separator — illegal in ns/key (we validate)
	schemaVersion   = 1
	defaultHnswM    = 16
	defaultEfSearch = 50
)

// SQLiteStore persists memory entries in a SQLite database and keeps an
// in-memory HNSW index for vector search. Thread-safe: all operations
// take a mutex before touching SQLite or the index.
//
// The HNSW index is rebuilt from SQLite on Open. Crash recovery is
// therefore "durable on disk, ephemeral in RAM" — no index files are
// written separately, which keeps the on-disk footprint simple.
type SQLiteStore struct {
	db       *sql.DB
	embedder EmbeddingProvider

	mu    sync.Mutex
	index *hnsw.Graph[indexNode]
	// nodes mirrors the HNSW index for O(1) (ns, key) -> Entry lookup
	// during Search result assembly. Map key is the compositeKey().
	nodes map[string]indexNode
}

// indexNode satisfies hnsw.Embeddable — the HNSW library needs each
// record to carry its own vector alongside a stable id.
type indexNode struct {
	compKey string
	ns      string
	key     string
	content string
	meta    map[string]string
	vec     []float32
}

func (n indexNode) ID() string           { return n.compKey }
func (n indexNode) Embedding() []float32 { return n.vec }

// OpenSQLiteStore opens (or creates) a SQLite database at dbPath and
// initialises the schema + HNSW index. The embedder's Dimensions()
// decides the index's vector size; mismatches against a pre-existing
// database are a hard error to prevent silent corruption.
func OpenSQLiteStore(ctx context.Context, dbPath string, embedder EmbeddingProvider) (*SQLiteStore, error) {
	if embedder == nil {
		return nil, ErrInvalid{Reason: "embedder must not be nil"}
	}
	if embedder.Dimensions() <= 0 {
		return nil, ErrInvalid{Reason: "embedder.Dimensions() must be > 0"}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite at %q: %w", dbPath, err)
	}
	// Busy timeout + WAL for decent concurrent-read behaviour even under
	// adb's mostly-single-writer pattern.
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite pragmas: %w", err)
	}

	s := &SQLiteStore{
		db:       db,
		embedder: embedder,
		nodes:    make(map[string]indexNode),
	}
	if err := s.initSchema(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.verifyOrRecordEmbedderMeta(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.rebuildIndex(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) initSchema(ctx context.Context) error {
	const ddl = `
create table if not exists memory_entries (
    namespace   text    not null,
    entry_key   text    not null,
    content     text    not null,
    meta_json   text    not null default '{}',
    embedding   blob    not null,
    created_at  text    not null,
    updated_at  text    not null,
    primary key (namespace, entry_key)
);
create table if not exists memory_metadata (
    k text primary key,
    v text not null
);
`
	if _, err := s.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("initSchema: %w", err)
	}
	return nil
}

// verifyOrRecordEmbedderMeta stores the embedder name + dimensions the
// first time the database is used, and refuses to proceed if a later
// caller tries to open with a different embedder (which would corrupt
// the index).
func (s *SQLiteStore) verifyOrRecordEmbedderMeta(ctx context.Context) error {
	var storedName, storedDim string
	err := s.db.QueryRowContext(ctx, `select v from memory_metadata where k = 'embedder_name'`).Scan(&storedName)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read embedder metadata: %w", err)
	}
	if err == sql.ErrNoRows {
		// First open — record and move on.
		_, err := s.db.ExecContext(ctx, `
insert into memory_metadata(k, v) values ('embedder_name', ?), ('embedder_dim', ?), ('schema_version', ?)
`, s.embedder.Name(), fmt.Sprintf("%d", s.embedder.Dimensions()), fmt.Sprintf("%d", schemaVersion))
		if err != nil {
			return fmt.Errorf("record embedder metadata: %w", err)
		}
		return nil
	}
	if err := s.db.QueryRowContext(ctx, `select v from memory_metadata where k = 'embedder_dim'`).Scan(&storedDim); err != nil {
		return fmt.Errorf("read embedder_dim: %w", err)
	}
	if storedName != s.embedder.Name() {
		return fmt.Errorf("embedder mismatch: store was created with %q, opened with %q — rebuilding the store with a new embedder is a destructive operation; delete the db file to recreate", storedName, s.embedder.Name())
	}
	if storedDim != fmt.Sprintf("%d", s.embedder.Dimensions()) {
		return fmt.Errorf("embedder dim mismatch: store has %s, embedder has %d", storedDim, s.embedder.Dimensions())
	}
	return nil
}

// rebuildIndex loads all entries from SQLite into s.nodes and constructs
// a fresh HNSW. Called once on Open. Deterministic RNG so the same input
// produces the same graph (reproducible tests).
func (s *SQLiteStore) rebuildIndex(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `select namespace, entry_key, content, meta_json, embedding from memory_entries`)
	if err != nil {
		return fmt.Errorf("rebuildIndex query: %w", err)
	}
	defer rows.Close()

	nodes := make(map[string]indexNode)
	for rows.Next() {
		var ns, key, content, metaJSON string
		var embBlob []byte
		if err := rows.Scan(&ns, &key, &content, &metaJSON, &embBlob); err != nil {
			return fmt.Errorf("rebuildIndex scan: %w", err)
		}
		vec, err := decodeVector(embBlob)
		if err != nil {
			return fmt.Errorf("decode embedding for (%s, %s): %w", ns, key, err)
		}
		meta := map[string]string{}
		if len(metaJSON) > 0 {
			if err := json.Unmarshal([]byte(metaJSON), &meta); err != nil {
				return fmt.Errorf("decode meta for (%s, %s): %w", ns, key, err)
			}
		}
		n := indexNode{
			compKey: compositeKey(ns, key),
			ns:      ns,
			key:     key,
			content: content,
			meta:    meta,
			vec:     vec,
		}
		nodes[n.compKey] = n
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rebuildIndex rows.Err: %w", err)
	}
	s.nodes = nodes
	s.rebuildIndexFromNodesLocked()
	return nil
}

// Upsert implements Store.
func (s *SQLiteStore) Upsert(ctx context.Context, ns, key, content string, meta map[string]string) error {
	if err := validateUpsert(ns, key); err != nil {
		return err
	}
	if strings.Contains(ns, compositeKeySep) || strings.Contains(key, compositeKeySep) {
		return ErrInvalid{Reason: "namespace and key must not contain ASCII record separator (U+001E)"}
	}

	vec, err := s.embedder.Embed(ctx, content)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	if len(vec) != s.embedder.Dimensions() {
		return fmt.Errorf("embedder returned vector of length %d, expected %d", len(vec), s.embedder.Dimensions())
	}
	embBlob := encodeVector(vec)
	metaJSON := "{}"
	if len(meta) > 0 {
		b, err := json.Marshal(meta)
		if err != nil {
			return fmt.Errorf("marshal meta: %w", err)
		}
		metaJSON = string(b)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Upsert via ON CONFLICT to preserve created_at on updates.
	_, err = s.db.ExecContext(ctx, `
insert into memory_entries (namespace, entry_key, content, meta_json, embedding, created_at, updated_at)
values (?, ?, ?, ?, ?, ?, ?)
on conflict (namespace, entry_key) do update set
    content = excluded.content,
    meta_json = excluded.meta_json,
    embedding = excluded.embedding,
    updated_at = excluded.updated_at
`, ns, key, content, metaJSON, embBlob, now, now)
	if err != nil {
		return fmt.Errorf("upsert sqlite: %w", err)
	}

	// Replace the authoritative entry in the in-memory map, then rebuild
	// the HNSW index from it. coder/hnsw v0.2.0 has no Delete or Update
	// API — re-adding an existing ID panics — so full rebuild is the
	// only correctness-safe path for mutations. For adb's workload
	// (tens to hundreds of records per workspace) rebuild is cheap
	// (O(N log N)); at much larger scale we'd need a library with
	// incremental update support or the v0.6.x API once its
	// renameio import is fixed upstream.
	n := indexNode{
		compKey: compositeKey(ns, key),
		ns:      ns,
		key:     key,
		content: content,
		meta:    meta,
		vec:     vec,
	}
	s.nodes[n.compKey] = n
	s.rebuildIndexFromNodesLocked()
	return nil
}

// rebuildIndexFromNodesLocked reconstructs the HNSW index from the
// current in-memory map. Caller must hold s.mu.
func (s *SQLiteStore) rebuildIndexFromNodesLocked() {
	g := hnsw.NewGraph[indexNode]()
	g.M = defaultHnswM
	g.EfSearch = defaultEfSearch
	g.Rng = rand.New(rand.NewSource(1))
	if len(s.nodes) > 0 {
		batch := make([]indexNode, 0, len(s.nodes))
		for _, n := range s.nodes {
			batch = append(batch, n)
		}
		g.Add(batch...)
	}
	s.index = g
}

// Search implements Store.
func (s *SQLiteStore) Search(ctx context.Context, ns, query string, k int) ([]Hit, error) {
	if ns == "" {
		return nil, ErrInvalid{Reason: "namespace must not be empty"}
	}
	if k <= 0 {
		k = 5
	}

	vec, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// HNSW has no per-namespace filter; we over-fetch and filter in Go.
	// `k * 10` is a heuristic: enough to reliably surface namespace hits
	// without over-walking the graph. If we still end up short, we fall
	// back to scanning `nodes` directly — correctness first, latency
	// second.
	want := k * 10
	if want < 50 {
		want = 50
	}
	graphHits := s.index.Search(vec, want)

	seen := map[string]struct{}{}
	out := make([]Hit, 0, k)
	for _, h := range graphHits {
		compKey := h.ID()
		if _, dup := seen[compKey]; dup {
			continue
		}
		node, ok := s.nodes[compKey]
		if !ok {
			// Orphan from a stale Upsert — skip.
			continue
		}
		if node.ns != ns {
			continue
		}
		seen[compKey] = struct{}{}
		out = append(out, Hit{
			Namespace: node.ns,
			Key:       node.key,
			Score:     cosineSimilarity(vec, node.vec),
			Content:   node.content,
			Meta:      copyMeta(node.meta),
		})
		if len(out) >= k {
			break
		}
	}

	// Fallback: if the graph-walk missed enough namespace-matching
	// candidates (small namespace drowned by larger ones), brute-force
	// scan `nodes` to fill up to k.
	if len(out) < k {
		scored := make([]Hit, 0)
		for compKey, node := range s.nodes {
			if node.ns != ns {
				continue
			}
			if _, already := seen[compKey]; already {
				continue
			}
			scored = append(scored, Hit{
				Namespace: node.ns,
				Key:       node.key,
				Score:     cosineSimilarity(vec, node.vec),
				Content:   node.content,
				Meta:      copyMeta(node.meta),
			})
		}
		sort.Slice(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })
		for _, h := range scored {
			out = append(out, h)
			if len(out) >= k {
				break
			}
		}
	}

	// Final sort so HNSW's greedy ordering and the fallback's exact
	// ordering blend correctly (descending score).
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, nil
}

// Delete implements Store.
func (s *SQLiteStore) Delete(ctx context.Context, ns, key string) error {
	if err := validateUpsert(ns, key); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, `delete from memory_entries where namespace = ? and entry_key = ?`, ns, key)
	if err != nil {
		return fmt.Errorf("delete sqlite: %w", err)
	}
	delete(s.nodes, compositeKey(ns, key))
	// Same rationale as Upsert: HNSW v0.2.0 has no Delete API, so the
	// safe path is a full rebuild. Cheap at adb's scale.
	s.rebuildIndexFromNodesLocked()
	return nil
}

// ListNamespaces implements Store.
func (s *SQLiteStore) ListNamespaces(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `select distinct namespace from memory_entries order by namespace`)
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var ns string
		if err := rows.Scan(&ns); err != nil {
			return nil, fmt.Errorf("scan namespace: %w", err)
		}
		out = append(out, ns)
	}
	return out, rows.Err()
}

// Close implements Store.
func (s *SQLiteStore) Close() error {
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

// compositeKey forms the HNSW.ID() for a (namespace, key) pair. Uses the
// ASCII record separator (U+001E) as delimiter because it is extremely
// unlikely in user input; Upsert validates that neither side contains it.
func compositeKey(ns, key string) string { return ns + compositeKeySep + key }

// encodeVector serialises a float32 slice as little-endian bytes so
// SQLite stores it as a compact BLOB.
func encodeVector(v []float32) []byte {
	buf := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(x))
	}
	return buf
}

// decodeVector inverse of encodeVector.
func decodeVector(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("decodeVector: blob length %d not a multiple of 4", len(b))
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v, nil
}

// cosineSimilarity returns 1.0 for identical directions, 0.0 for
// orthogonal, -1.0 for opposite. We surface it as the Score (higher is
// more similar).
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		da, db := float64(a[i]), float64(b[i])
		dot += da * db
		na += da * da
		nb += db * db
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(na) * math.Sqrt(nb)))
}

// copyMeta returns a shallow copy so callers can't mutate the store's
// internal state via a returned Hit.
func copyMeta(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
