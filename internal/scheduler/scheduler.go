// Package scheduler provides an in-binary job scheduler for adb.
//
// Jobs are registered by name and invoked on their own interval by a
// single goroutine per job. Per-job state (last run, duration, error)
// persists to disk so `adb scheduler list` survives restarts. The
// package avoids depending on other internal packages beyond the
// standard library — job implementations live in sibling files and
// receive a Deps struct at construction time.
package scheduler

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// Job is a scheduled unit of work.
type Job struct {
	Name            string
	DefaultInterval time.Duration
	Run             func(ctx context.Context) error
}

// JobConfig overrides a job's default interval or disables it entirely.
type JobConfig struct {
	Enabled  bool
	Interval time.Duration
}

// RunOptions configures Scheduler.Run.
type RunOptions struct {
	// Jobs to run. Each is invoked on its own interval.
	Jobs []Job
	// Config overrides, keyed by Job.Name. Missing entries use
	// Job.DefaultInterval and are enabled.
	Config map[string]JobConfig
	// StateFile is the path to the persisted job-state file. May be "".
	StateFile string
	// Logger receives one line per lifecycle event. May be nil.
	Logger io.Writer
	// Now returns the current time. Defaults to time.Now. Injected for tests.
	Now func() time.Time
	// RunOnStart invokes each enabled job once at startup before its
	// first tick. Defaults true for the daemon; tests override.
	RunOnStart bool
}

// State captures the last-run info for a job.
type State struct {
	Name         string        `yaml:"name"`
	LastStart    time.Time     `yaml:"last_start,omitempty"`
	LastEnd      time.Time     `yaml:"last_end,omitempty"`
	LastDuration time.Duration `yaml:"last_duration,omitempty"`
	LastError    string        `yaml:"last_error,omitempty"`
	Runs         int           `yaml:"runs"`
	Failures     int           `yaml:"failures"`
	Skipped      int           `yaml:"skipped"`
}

// Run starts the scheduler and blocks until ctx is cancelled. Returns
// ctx.Err() when ctx is done.
func Run(ctx context.Context, opts RunOptions) error {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	logf := func(format string, args ...interface{}) {
		if opts.Logger == nil {
			return
		}
		ts := opts.Now().UTC().Format(time.RFC3339)
		fmt.Fprintf(opts.Logger, "[%s] "+format+"\n", append([]interface{}{ts}, args...)...)
	}

	states := newStateStore(opts.StateFile)
	_ = states.load() // ignore errors on first start

	var wg sync.WaitGroup
	for i := range opts.Jobs {
		job := opts.Jobs[i]
		cfg, ok := opts.Config[job.Name]
		if ok && !cfg.Enabled {
			logf("job %s disabled by config, skipping", job.Name)
			continue
		}
		interval := job.DefaultInterval
		if ok && cfg.Interval > 0 {
			interval = cfg.Interval
		}
		if interval <= 0 {
			logf("job %s has zero interval, skipping", job.Name)
			continue
		}

		wg.Add(1)
		go func(j Job, iv time.Duration) {
			defer wg.Done()
			runJobLoop(ctx, j, iv, states, opts.Now, logf, opts.RunOnStart)
		}(job, interval)
	}

	wg.Wait()
	return ctx.Err()
}

func runJobLoop(
	ctx context.Context,
	job Job,
	interval time.Duration,
	states *stateStore,
	now func() time.Time,
	logf func(format string, args ...interface{}),
	runOnStart bool,
) {
	logf("job %s scheduled every %s", job.Name, interval)

	var (
		mu       sync.Mutex     // prevents overlap; skipped ticks increment Skipped
		inFlight sync.WaitGroup // tracks goroutine-spawned invocations so shutdown is clean
	)

	invoke := func() {
		if !mu.TryLock() {
			states.update(job.Name, func(s *State) { s.Skipped++ })
			logf("job %s skipped (previous run still in progress)", job.Name)
			return
		}
		defer mu.Unlock()

		start := now()
		states.update(job.Name, func(s *State) {
			s.Name = job.Name
			s.LastStart = start
		})
		logf("job %s started", job.Name)

		var runErr error
		func() {
			defer func() {
				if r := recover(); r != nil {
					runErr = fmt.Errorf("panic: %v", r)
				}
			}()
			runErr = job.Run(ctx)
		}()

		end := now()
		duration := end.Sub(start)
		states.update(job.Name, func(s *State) {
			s.LastEnd = end
			s.LastDuration = duration
			s.Runs++
			if runErr != nil {
				s.Failures++
				s.LastError = runErr.Error()
			} else {
				s.LastError = ""
			}
		})
		if runErr != nil {
			logf("job %s failed after %s: %v", job.Name, duration, runErr)
		} else {
			logf("job %s finished in %s", job.Name, duration)
		}
	}

	if runOnStart {
		// Run synchronously so a one-shot invocation pattern (used in
		// tests with a very short deadline) still sees at least one run.
		invoke()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			inFlight.Wait()
			return
		case <-ticker.C:
			// Spawn the invocation so a long-running job doesn't block
			// subsequent ticks from registering as overlap-skips.
			inFlight.Add(1)
			go func() {
				defer inFlight.Done()
				invoke()
			}()
		}
	}
}
