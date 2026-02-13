package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/drapaimern/ai-dev-brain/internal/core"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

// fakeFeedbackLoop implements core.FeedbackLoopOrchestrator for testing.
type fakeFeedbackLoop struct {
	runOpts   core.RunOptions
	runResult *core.LoopResult
	runErr    error
}

func (f *fakeFeedbackLoop) Run(opts core.RunOptions) (*core.LoopResult, error) {
	f.runOpts = opts
	return f.runResult, f.runErr
}

func (f *fakeFeedbackLoop) ProcessItem(_ models.ChannelItem) (*core.ProcessedItem, error) {
	return nil, nil
}

func TestLoopCmd(t *testing.T) {
	tests := []struct {
		name           string
		feedback       *fakeFeedbackLoop
		setNil         bool
		dryRun         bool
		channel        string
		wantErr        bool
		wantErrContain string
	}{
		{
			name: "successful run displays completed counts",
			feedback: &fakeFeedbackLoop{
				runResult: &core.LoopResult{
					ItemsFetched:     10,
					ItemsProcessed:   8,
					OutputsDelivered: 3,
					KnowledgeAdded:   2,
					Skipped:          2,
				},
			},
		},
		{
			name:   "dry run flag passes DryRun option",
			dryRun: true,
			feedback: &fakeFeedbackLoop{
				runResult: &core.LoopResult{
					ItemsFetched:     5,
					ItemsProcessed:   4,
					OutputsDelivered: 1,
					KnowledgeAdded:   0,
					Skipped:          1,
				},
			},
		},
		{
			name:    "channel filter passes ChannelFilter option",
			channel: "slack",
			feedback: &fakeFeedbackLoop{
				runResult: &core.LoopResult{
					ItemsFetched: 2,
				},
			},
		},
		{
			name: "run with warnings displays errors section",
			feedback: &fakeFeedbackLoop{
				runResult: &core.LoopResult{
					ItemsFetched:   3,
					ItemsProcessed: 1,
					Errors:         []string{"fetching from email: timeout", "processing item x: parse error"},
				},
			},
		},
		{
			name:           "nil FeedbackLoop returns error",
			setNil:         true,
			wantErr:        true,
			wantErrContain: "feedback loop not initialized",
		},
		{
			name: "Run returns error wraps with context",
			feedback: &fakeFeedbackLoop{
				runErr: errors.New("channel registry empty"),
			},
			wantErr:        true,
			wantErrContain: "running feedback loop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore package-level variable.
			origFeedbackLoop := FeedbackLoop
			origDryRun := loopDryRun
			origChannel := loopChannel
			defer func() {
				FeedbackLoop = origFeedbackLoop
				loopDryRun = origDryRun
				loopChannel = origChannel
			}()

			if tt.setNil {
				FeedbackLoop = nil
			} else {
				FeedbackLoop = tt.feedback
			}

			loopDryRun = tt.dryRun
			loopChannel = tt.channel

			err := loopCmd.RunE(loopCmd, []string{})

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Errorf("expected error containing %q, got %q", tt.wantErrContain, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify options were passed correctly.
			if tt.feedback.runOpts.DryRun != tt.dryRun {
				t.Errorf("expected DryRun=%v, got %v", tt.dryRun, tt.feedback.runOpts.DryRun)
			}
			if tt.feedback.runOpts.ChannelFilter != tt.channel {
				t.Errorf("expected ChannelFilter=%q, got %q", tt.channel, tt.feedback.runOpts.ChannelFilter)
			}
		})
	}
}
