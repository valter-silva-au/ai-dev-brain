package cli

import (
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/integration"
)

// TestSchedulerPullRepos_FetchOnly guards the unattended repos-pull job's
// semantics: `adb repos pull` checks out the default branch (checkout -B),
// which is right for an interactive run but wrong for a background job that
// fires every 15 minutes — it would silently switch a clean repo off a
// deliberately checked-out branch. The scheduler must therefore fetch only;
// checkouts stay a deliberate, interactive act.
func TestSchedulerPullRepos_FetchOnly(t *testing.T) {
	var got integration.PullOpts
	called := false
	_, err := schedulerPullRepos("/base", func(basePath string, opts integration.PullOpts) (integration.PullSummary, error) {
		called = true
		if basePath != "/base" {
			t.Errorf("basePath = %q, want /base", basePath)
		}
		got = opts
		return integration.PullSummary{}, nil
	})
	if err != nil {
		t.Fatalf("schedulerPullRepos: %v", err)
	}
	if !called {
		t.Fatal("pull engine was never invoked")
	}
	if !got.FetchOnly {
		t.Error("scheduler repos-pull job must be FetchOnly — an unattended job may never switch branches")
	}
}
