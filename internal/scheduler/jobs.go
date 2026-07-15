package scheduler

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Deps bundles the handles jobs need. Concrete implementations are
// injected from cmd/cli so this package stays dependency-free.
type Deps struct {
	BasePath string
	Logger   io.Writer

	// PullRepos runs an `adb repos pull` once. Should return a short
	// one-line summary suitable for logging. Never returns nil unless err.
	PullRepos func(ctx context.Context) (string, error)

	// EvaluateAlerts runs AlertEvaluator and returns the alert count and
	// a rendered summary suitable for logging.
	EvaluateAlerts func(ctx context.Context) (int, string, error)

	// LogFiles is the set of files the events-rotate job should size-check.
	// Usually the adb events log and the scheduler's own log file.
	LogFiles []string
}

// DefaultJobs constructs the three v1 jobs from the supplied Deps.
func DefaultJobs(deps Deps) []Job {
	return []Job{
		{
			Name:            "repos-pull",
			DefaultInterval: 15 * time.Minute,
			Run: func(ctx context.Context) error {
				if deps.PullRepos == nil {
					return fmt.Errorf("repos-pull: no PullRepos handler wired")
				}
				summary, err := deps.PullRepos(ctx)
				if err != nil {
					return err
				}
				fmt.Fprintf(deps.Logger, "    %s\n", summary)
				return nil
			},
		},
		{
			Name:            "alerts-tick",
			DefaultInterval: 1 * time.Hour,
			Run: func(ctx context.Context) error {
				if deps.EvaluateAlerts == nil {
					return fmt.Errorf("alerts-tick: no EvaluateAlerts handler wired")
				}
				count, summary, err := deps.EvaluateAlerts(ctx)
				if err != nil {
					return err
				}
				fmt.Fprintf(deps.Logger, "    %d alerts active\n%s", count, summary)
				return nil
			},
		},
		{
			Name:            "events-rotate",
			DefaultInterval: 6 * time.Hour,
			Run: func(ctx context.Context) error {
				for _, path := range deps.LogFiles {
					if path == "" {
						continue
					}
					rotated, err := rotateIfLarge(path, rotateThreshold, keepRotations)
					if err != nil {
						fmt.Fprintf(deps.Logger, "    rotate %s: %v\n", path, err)
						continue
					}
					if rotated {
						fmt.Fprintf(deps.Logger, "    rotated %s\n", path)
					}
				}
				return nil
			},
		},
	}
}

const (
	rotateThreshold = 50 * 1024 * 1024 // 50 MiB
	keepRotations   = 3
)

// rotateIfLarge rotates path to path.1 (shifting .1 → .2, ...) if its
// size is >= threshold. Returns true if rotation happened. Siblings
// beyond keep are deleted.
func rotateIfLarge(path string, threshold int64, keep int) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.Size() < threshold {
		return false, nil
	}

	// Shift existing rotations: .{keep-1} -> .keep (dropped), ... .1 -> .2
	for i := keep; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", path, i)
		if i == keep {
			// Oldest rotation — delete if present.
			if _, err := os.Stat(src); err == nil {
				_ = os.Remove(src)
			}
			continue
		}
		dst := fmt.Sprintf("%s.%d", path, i+1)
		if _, err := os.Stat(src); err == nil {
			if err := os.Rename(src, dst); err != nil {
				return false, fmt.Errorf("shift %s -> %s: %w", src, dst, err)
			}
		}
	}
	// Move current file to .1
	if err := os.Rename(path, path+".1"); err != nil {
		return false, fmt.Errorf("rotate %s: %w", path, err)
	}
	// Recreate an empty file so subsequent appenders don't break.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, fmt.Errorf("recreate %s: %w", path, err)
	}
	_ = f.Close()
	return true, nil
}

// EnsureDir makes sure the parent of p exists. Small helper used when
// wiring log files.
func EnsureDir(p string) error {
	return os.MkdirAll(filepath.Dir(p), 0o755)
}
