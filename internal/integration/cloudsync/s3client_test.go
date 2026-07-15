package cloudsync

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

// fakeStore is an in-memory ObjectStore used across the cloudsync tests.
// The test that lives here proves the seam: sync.go / sync_test.go depend
// only on the ObjectStore interface, so orchestration is testable offline
// (no AWS account, no network, no credentials).
type fakeStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newFakeStore() *fakeStore { return &fakeStore{data: map[string][]byte{}} }

func (f *fakeStore) Put(_ context.Context, key string, body io.Reader) error {
	buf, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key] = buf
	return nil
}

func (f *fakeStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	buf, ok := f.data[key]
	if !ok {
		return nil, errors.New("no such key: " + key)
	}
	return io.NopCloser(bytes.NewReader(buf)), nil
}

func (f *fakeStore) List(_ context.Context, prefix string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for k := range f.data {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (f *fakeStore) Delete(_ context.Context, keys []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, k := range keys {
		delete(f.data, k)
	}
	return nil
}

// TestObjectStoreSeam pins the interface: any store — real S3 or fake —
// must be usable through the same shape (Put / Get / List / Delete).
// This is the interface seam the orchestrator depends on, not *S3Store.
func TestObjectStoreSeam(t *testing.T) {
	var store ObjectStore = newFakeStore()
	ctx := context.Background()

	if err := store.Put(ctx, "raw/a.md", strings.NewReader("hello")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Put(ctx, "wiki/b.md", strings.NewReader("world")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	keys, err := store.List(ctx, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !reflect.DeepEqual(keys, []string{"raw/a.md", "wiki/b.md"}) {
		t.Errorf("List: %v", keys)
	}

	rc, err := store.Get(ctx, "raw/a.md")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()
	body, _ := io.ReadAll(rc)
	if string(body) != "hello" {
		t.Errorf("Get body = %q", body)
	}

	prefix, err := store.List(ctx, "raw/")
	if err != nil {
		t.Fatalf("List prefix: %v", err)
	}
	if !reflect.DeepEqual(prefix, []string{"raw/a.md"}) {
		t.Errorf("List prefix mismatch: %v", prefix)
	}

	if err := store.Delete(ctx, []string{"raw/a.md"}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	after, _ := store.List(ctx, "")
	if !reflect.DeepEqual(after, []string{"wiki/b.md"}) {
		t.Errorf("after Delete: %v", after)
	}
}
