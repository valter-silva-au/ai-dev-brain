package foundation

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

// TestInitializeReportsUnrecordableFailure covers what failInitialize used to
// throw away.
//
// failInitialize appends a `failed` phase to the operation journal and then
// returns the cause. That append's error was discarded (`_, _ =`), and the loss
// is not cosmetic: with no failed phase on record the journal folds to
// `applying`, so `adb doctor` reports operation.journal.incomplete — "resume
// only when its target state is unambiguous" — for an operation that actually
// failed and must not be resumed. Nothing else in the process can tell the
// caller that, because the returned error is the helper's only channel.
//
// The double fault is provoked by an after-plan hook that (a) removes the
// operation's journal directory, so the failure cannot be recorded, and
// (b) plants a regular file where the cache directory has to be created, so
// there IS a failure to record.
func TestInitializeReportsUnrecordableFailure(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "AWS")
	layout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}

	service := newTestService(t, func() error {
		if removeErr := os.RemoveAll(layout.EventsDir()); removeErr != nil {
			return removeErr
		}
		return os.WriteFile(
			layout.CacheDir(),
			[]byte("a file where a directory belongs"),
			0o644,
		)
	})

	result, err := service.Initialize(context.Background(), InitializeRequest{
		Root:  root,
		Name:  "AWS",
		Apply: true,
	})
	if err == nil {
		t.Fatal("initialize succeeded despite an unusable cache path")
	}
	if result.Outcome != capability.OutcomeFailed {
		t.Fatalf("outcome = %q, want failed", result.Outcome)
	}

	message := err.Error()
	// The cause stays primary — the fold must not bury what actually went wrong.
	if !strings.Contains(message, "cache directory") {
		t.Errorf("error %q no longer names the primary cause", message)
	}
	if !strings.Contains(message, "operation journal") {
		t.Errorf(
			"error %q does not report that the failure could not be recorded in "+
				"the operation journal; adb doctor will read this operation as "+
				"incomplete rather than failed",
			message,
		)
	}
	if !strings.Contains(message, result.Data.OperationID) {
		t.Errorf(
			"error %q does not name operation %q",
			message,
			result.Data.OperationID,
		)
	}
	if strings.Contains(message, "\n") {
		t.Errorf("error %q spans lines; it is rendered inline by the CLI", message)
	}
}
