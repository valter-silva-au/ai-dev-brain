package cli

import (
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestValidateTaskType(t *testing.T) {
	valid := map[string]models.TaskType{
		"feat":     models.TaskTypeFeat,
		"fix":      models.TaskTypeFix,
		"refactor": models.TaskTypeRefactor,
		"docs":     models.TaskTypeDocs,
		"chore":    models.TaskTypeChore,
		"test":     models.TaskTypeTest,
		"perf":     models.TaskTypePerf,
		"spike":    models.TaskTypeSpike,
	}
	for in, want := range valid {
		t.Run("valid/"+in, func(t *testing.T) {
			got, err := validateTaskType(in)
			if err != nil {
				t.Fatalf("validateTaskType(%q) unexpected error: %v", in, err)
			}
			if got != want {
				t.Errorf("validateTaskType(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

// TestValidateTaskType_BugRejectedWithHint: the legacy "bug" type is no longer
// accepted at create time; the error must steer the caller to "fix".
func TestValidateTaskType_BugRejectedWithHint(t *testing.T) {
	_, err := validateTaskType("bug")
	if err == nil {
		t.Fatal("validateTaskType(\"bug\") = nil error, want a rejection")
	}
	if !strings.Contains(err.Error(), "fix") {
		t.Errorf("bug rejection %q should hint to use `fix`", err.Error())
	}
}

func TestValidateTaskType_UnknownRejected(t *testing.T) {
	if _, err := validateTaskType("banana"); err == nil {
		t.Error("validateTaskType(\"banana\") = nil error, want a rejection")
	}
}
