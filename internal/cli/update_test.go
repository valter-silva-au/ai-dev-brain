package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/drapaimern/ai-dev-brain/internal/core"
)

// mockUpdateGenerator implements core.UpdateGenerator for testing.
type mockUpdateGenerator struct {
	generateUpdatesFn func(taskID string) (*core.UpdatePlan, error)
}

func (m *mockUpdateGenerator) GenerateUpdates(taskID string) (*core.UpdatePlan, error) {
	if m.generateUpdatesFn != nil {
		return m.generateUpdatesFn(taskID)
	}
	return &core.UpdatePlan{
		TaskID:      taskID,
		GeneratedAt: time.Now(),
	}, nil
}

func TestUpdateCommand_Registration(t *testing.T) {
	subcommands := rootCmd.Commands()
	found := false
	for _, cmd := range subcommands {
		if cmd.Name() == "update" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'update' command to be registered")
	}
}

func TestUpdateCommand_NilGenerator(t *testing.T) {
	origGen := UpdateGen
	defer func() { UpdateGen = origGen }()
	UpdateGen = nil

	var buf bytes.Buffer
	updateCmd.SetArgs([]string{"TASK-00001"})
	updateCmd.SetOut(&buf)
	updateCmd.SetErr(&buf)

	err := updateCmd.RunE(updateCmd, []string{"TASK-00001"})
	if err == nil {
		t.Fatal("expected error when UpdateGen is nil")
	}
	if !strings.Contains(err.Error(), "update generator not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUpdateCommand_GeneratesOutput(t *testing.T) {
	origGen := UpdateGen
	defer func() { UpdateGen = origGen }()

	UpdateGen = &mockUpdateGenerator{
		generateUpdatesFn: func(taskID string) (*core.UpdatePlan, error) {
			return &core.UpdatePlan{
				TaskID:      taskID,
				GeneratedAt: time.Now(),
				Messages: []core.PlannedMessage{
					{
						Recipient: "@alice",
						Reason:    "Progress update",
						Channel:   core.ChannelSlack,
						Subject:   "Update: " + taskID,
						Body:      "Here is the update.",
						Priority:  core.MsgPriorityNormal,
					},
				},
				InformationNeeded: []core.InformationRequest{
					{
						From:     "@bob",
						Question: "What is the deadline?",
						Context:  "Blocking issue",
						Blocking: true,
					},
				},
			}, nil
		},
	}

	err := updateCmd.RunE(updateCmd, []string{"TASK-00042"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateCommand_GenerateError(t *testing.T) {
	origGen := UpdateGen
	defer func() { UpdateGen = origGen }()

	UpdateGen = &mockUpdateGenerator{
		generateUpdatesFn: func(taskID string) (*core.UpdatePlan, error) {
			return nil, fmt.Errorf("context not found")
		},
	}

	err := updateCmd.RunE(updateCmd, []string{"TASK-99999"})
	if err == nil {
		t.Fatal("expected error from GenerateUpdates")
	}
	if !strings.Contains(err.Error(), "context not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUpdateCommand_EmptyPlan(t *testing.T) {
	origGen := UpdateGen
	defer func() { UpdateGen = origGen }()

	UpdateGen = &mockUpdateGenerator{
		generateUpdatesFn: func(taskID string) (*core.UpdatePlan, error) {
			return &core.UpdatePlan{
				TaskID:      taskID,
				GeneratedAt: time.Now(),
			}, nil
		},
	}

	err := updateCmd.RunE(updateCmd, []string{"TASK-00001"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
