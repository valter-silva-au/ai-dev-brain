package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestAlerts_HasNoNotifyFlag pins the ABSENCE of `--notify`.
//
// The flag existed and did nothing: its block printed "Sending
// notifications..." and then "✓ Notifications sent" directly underneath a
// `// TODO: Implement notification sending`. There was no transport. A flag
// that reports success for work it never performed is worse than an absent
// flag, because the absent one cannot be believed.
//
// A real notification subsystem is credentials, a client per channel, retry,
// and per-channel failure reporting — a feature, not a follow-up. So this
// asserts the flag is gone rather than that it works.
func TestAlerts_HasNoNotifyFlag(t *testing.T) {
	cmd := NewAlertsCmd()

	if f := cmd.Flags().Lookup("notify"); f != nil {
		t.Errorf("adb alerts must not register a --notify flag; found %q (%s)", f.Name, f.Usage)
	}
	if f := cmd.LocalFlags().Lookup("notify"); f != nil {
		t.Errorf("adb alerts must not carry a local --notify flag; found %q", f.Name)
	}
}

// TestAlerts_RejectsNotifyFlag is the behavioural half: the spelling a user or
// script would actually type is refused by the flag parser, before RunE. The
// flag-registry check above could pass while some other seam still accepted
// the argument.
func TestAlerts_RejectsNotifyFlag(t *testing.T) {
	cmd := NewAlertsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"--notify"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("adb alerts --notify must be rejected as an unknown flag, got no error")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("want an unknown-flag error, got %v", err)
	}
}

// TestAlerts_HelpNeverAdvertisesNotifications guards the documentation surface.
// Help text is where a removed flag most easily survives, and a help entry for
// a flag that no longer exists is its own defect.
func TestAlerts_HelpNeverAdvertisesNotifications(t *testing.T) {
	cmd := NewAlertsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Help(); err != nil {
		t.Fatalf("rendering help: %v", err)
	}

	help := out.String()
	for _, banned := range []string{"notify", "Notifications sent", "notification"} {
		if strings.Contains(strings.ToLower(help), strings.ToLower(banned)) {
			t.Errorf("adb alerts help must not mention %q; got:\n%s", banned, help)
		}
	}
}
