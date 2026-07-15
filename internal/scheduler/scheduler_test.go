package scheduler

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestRun_InvokesJobOnTick(t *testing.T) {
	var calls int32
	job := Job{
		Name:            "spin",
		DefaultInterval: 50 * time.Millisecond,
		Run: func(ctx context.Context) error {
			atomic.AddInt32(&calls, 1)
			return nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Millisecond)
	defer cancel()

	if err := Run(ctx, RunOptions{Jobs: []Job{job}, RunOnStart: true}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}

	// Expected: one run-on-start plus at least 2 ticks within 180ms.
	got := atomic.LoadInt32(&calls)
	if got < 2 {
		t.Fatalf("expected >=2 invocations, got %d", got)
	}
}

func TestRun_DisabledJobSkipped(t *testing.T) {
	var calls int32
	job := Job{
		Name:            "off",
		DefaultInterval: 10 * time.Millisecond,
		Run: func(ctx context.Context) error {
			atomic.AddInt32(&calls, 1)
			return nil
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()

	_ = Run(ctx, RunOptions{
		Jobs:       []Job{job},
		Config:     map[string]JobConfig{"off": {Enabled: false}},
		RunOnStart: true,
	})
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("disabled job was invoked %d times", calls)
	}
}

func TestRun_PanicRecovered(t *testing.T) {
	var calls int32
	job := Job{
		Name:            "panicky",
		DefaultInterval: 30 * time.Millisecond,
		Run: func(ctx context.Context) error {
			n := atomic.AddInt32(&calls, 1)
			if n == 1 {
				panic("boom")
			}
			return nil
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	_ = Run(ctx, RunOptions{Jobs: []Job{job}, RunOnStart: true})
	if atomic.LoadInt32(&calls) < 2 {
		t.Fatalf("scheduler did not recover from panic; calls=%d", calls)
	}
}

func TestRun_OverlapSkipped(t *testing.T) {
	var started int32
	release := make(chan struct{})
	job := Job{
		Name:            "slow",
		DefaultInterval: 10 * time.Millisecond,
		Run: func(ctx context.Context) error {
			if atomic.AddInt32(&started, 1) == 1 {
				<-release
			}
			return nil
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.yaml")

	done := make(chan struct{})
	go func() {
		_ = Run(ctx, RunOptions{Jobs: []Job{job}, StateFile: stateFile, RunOnStart: false})
		close(done)
	}()

	// Wait for the first tick to fire (goroutine-spawned invoke()).
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) && atomic.LoadInt32(&started) < 1 {
		time.Sleep(2 * time.Millisecond)
	}
	if atomic.LoadInt32(&started) < 1 {
		t.Fatalf("first invocation never started")
	}

	// While the first run holds the mutex, several ticks should attempt
	// and fail to acquire it — each should bump Skipped.
	time.Sleep(80 * time.Millisecond)
	close(release)
	cancel()
	<-done

	states, err := LoadStates(stateFile)
	if err != nil {
		t.Fatalf("LoadStates: %v", err)
	}
	var s *State
	for i := range states {
		if states[i].Name == "slow" {
			s = &states[i]
			break
		}
	}
	if s == nil {
		t.Fatalf("no state recorded for slow job")
	}
	if s.Skipped == 0 {
		t.Fatalf("expected overlap skips to be recorded; got %+v", s)
	}
}

func TestStateStore_PersistAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")

	s := newStateStore(path)
	s.update("alpha", func(st *State) {
		st.Runs = 3
		st.LastDuration = 42 * time.Millisecond
	})

	got, err := LoadStates(path)
	if err != nil {
		t.Fatalf("LoadStates: %v", err)
	}
	if len(got) != 1 || got[0].Name != "alpha" || got[0].Runs != 3 {
		t.Fatalf("unexpected reloaded state: %+v", got)
	}
}
