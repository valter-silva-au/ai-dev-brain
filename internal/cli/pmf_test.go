package cli

import (
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
)

// TestPMFCLI_RecordListAndValueRequired drives `adb pmf` through the real App:
// --value is required (so a legitimate 0 still records), record upserts, and
// list renders the recorded metric.
func TestPMFCLI_RecordListAndValueRequired(t *testing.T) {
	tmp := t.TempDir()
	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	App = app

	// --value omitted → error (not a silent 0).
	if err := runADB(t, "pmf", "record", "--initiative", "onboarding", "--metric", "sean-ellis"); err == nil {
		t.Fatal("expected an error when --value is omitted")
	}

	// A legitimate 0 value records (Changed, not zero-value, is the gate).
	if err := runADB(t, "pmf", "record", "--initiative", "onboarding", "--metric", "churn", "--value", "0"); err != nil {
		t.Fatalf("record 0-value metric: %v", err)
	}
	if m, found, err := app.MetricStore.Get("onboarding", "churn"); err != nil || !found || m.Value != 0 {
		t.Fatalf("Get churn = %+v found %v err %v, want a recorded 0", m, found, err)
	}

	// Record + upsert sean-ellis, then list shows it with its provenance.
	if err := runADB(t, "pmf", "record", "--initiative", "onboarding", "--metric", "sean-ellis", "--value", "45", "--unit", "%"); err != nil {
		t.Fatalf("record sean-ellis: %v", err)
	}
	out, err := runADBOut(t, "pmf", "list", "--initiative", "onboarding")
	if err != nil {
		t.Fatalf("pmf list: %v", err)
	}
	if !strings.Contains(out, "sean-ellis") || !strings.Contains(out, "manual") {
		t.Fatalf("pmf list output missing metric/provenance:\n%s", out)
	}
}
