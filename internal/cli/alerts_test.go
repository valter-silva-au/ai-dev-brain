package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
)

type alertsMock struct {
	evaluateFn func() ([]observability.Alert, error)
}

func (m *alertsMock) Evaluate() ([]observability.Alert, error) {
	return m.evaluateFn()
}

type notifierMock struct {
	notifyFn func(alerts []observability.Alert) error
}

func (m *notifierMock) Notify(alerts []observability.Alert) error {
	return m.notifyFn(alerts)
}

func TestAlertsCmd_NilEngine(t *testing.T) {
	orig := AlertEngine
	defer func() { AlertEngine = orig }()
	AlertEngine = nil

	err := alertsCmd.RunE(alertsCmd, []string{})
	if err == nil {
		t.Fatal("expected error when AlertEngine is nil")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAlertsCmd_NoAlerts(t *testing.T) {
	orig := AlertEngine
	defer func() { AlertEngine = orig }()

	AlertEngine = &alertsMock{
		evaluateFn: func() ([]observability.Alert, error) {
			return nil, nil
		},
	}

	err := alertsCmd.RunE(alertsCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAlertsCmd_WithAlerts(t *testing.T) {
	orig := AlertEngine
	defer func() { AlertEngine = orig }()

	AlertEngine = &alertsMock{
		evaluateFn: func() ([]observability.Alert, error) {
			return []observability.Alert{
				{Severity: observability.SeverityHigh, Message: "task blocked", TriggeredAt: time.Now().UTC()},
				{Severity: observability.SeverityLow, Message: "backlog large", TriggeredAt: time.Now().UTC()},
			}, nil
		},
	}

	err := alertsCmd.RunE(alertsCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAlertsCmd_EvaluateError(t *testing.T) {
	orig := AlertEngine
	defer func() { AlertEngine = orig }()

	AlertEngine = &alertsMock{
		evaluateFn: func() ([]observability.Alert, error) {
			return nil, fmt.Errorf("event log read error")
		},
	}

	err := alertsCmd.RunE(alertsCmd, []string{})
	if err == nil {
		t.Fatal("expected error from Evaluate")
	}
	if !strings.Contains(err.Error(), "evaluating alerts") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAlertsCmd_NotifyWithoutNotifier(t *testing.T) {
	origEngine := AlertEngine
	origNotifier := Notifier
	defer func() {
		AlertEngine = origEngine
		Notifier = origNotifier
	}()

	AlertEngine = &alertsMock{
		evaluateFn: func() ([]observability.Alert, error) {
			return []observability.Alert{
				{Severity: observability.SeverityHigh, Message: "blocked", TriggeredAt: time.Now().UTC()},
			}, nil
		},
	}
	Notifier = nil

	// Set --notify flag on the command.
	alertsCmd.Flags().Set("notify", "true")
	defer alertsCmd.Flags().Set("notify", "false")

	err := alertsCmd.RunE(alertsCmd, []string{})
	if err == nil {
		t.Fatal("expected error when notifier is nil")
	}
	if !strings.Contains(err.Error(), "notifier not configured") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAlertsCmd_NotifySuccess(t *testing.T) {
	origEngine := AlertEngine
	origNotifier := Notifier
	defer func() {
		AlertEngine = origEngine
		Notifier = origNotifier
	}()

	AlertEngine = &alertsMock{
		evaluateFn: func() ([]observability.Alert, error) {
			return []observability.Alert{
				{Severity: observability.SeverityMedium, Message: "stale task", TriggeredAt: time.Now().UTC()},
			}, nil
		},
	}

	var notified bool
	Notifier = &notifierMock{
		notifyFn: func(alerts []observability.Alert) error {
			notified = true
			return nil
		},
	}

	alertsCmd.Flags().Set("notify", "true")
	defer alertsCmd.Flags().Set("notify", "false")

	err := alertsCmd.RunE(alertsCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !notified {
		t.Error("expected Notify to be called")
	}
}

func TestAlertsCmd_NotifyError(t *testing.T) {
	origEngine := AlertEngine
	origNotifier := Notifier
	defer func() {
		AlertEngine = origEngine
		Notifier = origNotifier
	}()

	AlertEngine = &alertsMock{
		evaluateFn: func() ([]observability.Alert, error) {
			return []observability.Alert{
				{Severity: observability.SeverityLow, Message: "large backlog", TriggeredAt: time.Now().UTC()},
			}, nil
		},
	}
	Notifier = &notifierMock{
		notifyFn: func(alerts []observability.Alert) error {
			return fmt.Errorf("webhook failed")
		},
	}

	alertsCmd.Flags().Set("notify", "true")
	defer alertsCmd.Flags().Set("notify", "false")

	err := alertsCmd.RunE(alertsCmd, []string{})
	if err == nil {
		t.Fatal("expected error from Notify")
	}
	if !strings.Contains(err.Error(), "sending notifications") {
		t.Errorf("unexpected error: %v", err)
	}
}
