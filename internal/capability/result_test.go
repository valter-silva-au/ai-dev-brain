package capability

import (
	"encoding/json"
	"testing"
)

func TestResultJSONContract(t *testing.T) {
	t.Parallel()

	result := Result[map[string]string]{
		Capability: "workspace.initialize",
		Version:    "v1",
		Outcome:    OutcomeApplied,
		Data:       map[string]string{"workspace_id": "workspace-1"},
		Effects: []Effect{{
			Action: "create",
			Target: ".aidb/manifest.yaml",
			Status: EffectApplied,
		}},
		Warnings: []Notice{},
		NextActions: []Action{{
			Code:    "run_doctor",
			Message: "Run adb doctor.",
		}},
		Recovery: Recovery{Required: false},
	}

	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	wantKeys := []string{
		"capability", "version", "outcome", "data", "effects",
		"warnings", "next_actions", "recovery",
	}
	for _, key := range wantKeys {
		if _, ok := got[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}
	if got["capability"] != "workspace.initialize" {
		t.Errorf("capability = %v", got["capability"])
	}
	if got["version"] != "v1" {
		t.Errorf("version = %v", got["version"])
	}
	if got["outcome"] != "applied" {
		t.Errorf("outcome = %v", got["outcome"])
	}
}
