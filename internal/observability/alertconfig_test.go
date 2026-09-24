package observability

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"
)

// staticLookup builds an AlertSettingLookup over a fixed key→(value,tier) map,
// standing in for models.MergedConfig.SettingSource.
func staticLookup(values map[string]string, tier string) AlertSettingLookup {
	return func(key string) (string, string, bool) {
		v, ok := values[key]
		if !ok {
			return "", "", false
		}
		return v, tier, true
	}
}

// TestResolveAlertConfig_UnsetEqualsDefaults is the defaults-do-not-move guard:
// a workspace that configures nothing must resolve to exactly DefaultAlertConfig(),
// field for field, so `adb alerts` behaves today as it did before thresholds
// became configurable. A nil lookup (no MergedConfig at all) must do the same.
func TestResolveAlertConfig_UnsetEqualsDefaults(t *testing.T) {
	t.Parallel()

	for name, lookup := range map[string]AlertSettingLookup{
		"nil lookup":   nil,
		"empty lookup": staticLookup(nil, "repo"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, warnings := ResolveAlertConfig(lookup)
			if len(warnings) != 0 {
				t.Errorf("unconfigured workspace produced warnings: %v", warnings)
			}
			if !reflect.DeepEqual(got, DefaultAlertConfig()) {
				t.Fatalf("resolved config = %+v, want DefaultAlertConfig() %+v", got, DefaultAlertConfig())
			}
		})
	}
}

// TestResolveAlertConfig_KeysAreDerivedFromAlertTypes pins the key spelling. The
// key is "alert_" + the AlertType string, so the config vocabulary cannot drift
// from the alert vocabulary. Underscored on purpose: Viper treats "." as a
// nesting delimiter and custom_settings is a flat map[string]string.
func TestResolveAlertConfig_KeysAreDerivedFromAlertTypes(t *testing.T) {
	t.Parallel()

	want := []string{
		"alert_task_blocked_too_long",
		"alert_task_stale",
		"alert_review_too_long",
		"alert_backlog_too_large",
	}
	if got := AlertSettingKeys(); !reflect.DeepEqual(got, want) {
		t.Fatalf("AlertSettingKeys() = %v, want %v", got, want)
	}
	for _, k := range AlertSettingKeys() {
		if strings.ContainsAny(k, ". ") {
			t.Errorf("key %q contains a dot or space; custom_settings keys must be flat and underscored", k)
		}
	}
	// One key per default threshold — a new threshold cannot be unconfigurable.
	if len(AlertSettingKeys()) != len(DefaultAlertConfig().Thresholds) {
		t.Fatalf("AlertSettingKeys() has %d keys, DefaultAlertConfig() has %d thresholds",
			len(AlertSettingKeys()), len(DefaultAlertConfig().Thresholds))
	}
	if got := AlertSettingKey(AlertTaskStale); got != "alert_task_stale" {
		t.Fatalf("AlertSettingKey(AlertTaskStale) = %q, want alert_task_stale", got)
	}
}

// TestResolveAlertConfig_EachKeySetsItsThreshold walks every configurable
// threshold: the configured value lands, the OTHER three keep their defaults,
// and the severity is untouched (severity is not configurable).
func TestResolveAlertConfig_EachKeySetsItsThreshold(t *testing.T) {
	t.Parallel()

	cases := []struct {
		key          string
		alertType    AlertType
		value        string
		wantDuration time.Duration
		wantCount    int
	}{
		{key: "alert_task_blocked_too_long", alertType: AlertTaskBlockedTooLong, value: "4h", wantDuration: 4 * time.Hour},
		{key: "alert_task_stale", alertType: AlertTaskStale, value: "10d", wantDuration: 10 * 24 * time.Hour},
		{key: "alert_review_too_long", alertType: AlertReviewTooLong, value: "36h", wantDuration: 36 * time.Hour},
		{key: "alert_backlog_too_large", alertType: AlertBacklogTooLarge, value: "42", wantCount: 42},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			t.Parallel()
			got, warnings := ResolveAlertConfig(staticLookup(map[string]string{tc.key: tc.value}, "repo"))
			if len(warnings) != 0 {
				t.Fatalf("%s=%q produced warnings: %v", tc.key, tc.value, warnings)
			}
			th := got.GetThreshold(tc.alertType)
			if th == nil {
				t.Fatalf("threshold %s missing after configuration", tc.alertType)
			}
			if th.Duration != tc.wantDuration {
				t.Errorf("%s duration = %v, want %v", tc.alertType, th.Duration, tc.wantDuration)
			}
			if th.Count != tc.wantCount {
				t.Errorf("%s count = %d, want %d", tc.alertType, th.Count, tc.wantCount)
			}
			// Severity comes from the defaults, never from config.
			wantSeverity := DefaultAlertConfig().GetThreshold(tc.alertType).Severity
			if th.Severity != wantSeverity {
				t.Errorf("%s severity = %q, want the default %q", tc.alertType, th.Severity, wantSeverity)
			}
			// Every other threshold is byte-identical to its default.
			for _, other := range DefaultAlertConfig().Thresholds {
				if other.Type == tc.alertType {
					continue
				}
				if resolved := got.GetThreshold(other.Type); resolved == nil || *resolved != other {
					t.Errorf("configuring %s changed %s: got %+v, want %+v", tc.key, other.Type, resolved, other)
				}
			}
		})
	}
}

// TestResolveAlertConfig_AllFourAtOnce proves the four keys are independent.
func TestResolveAlertConfig_AllFourAtOnce(t *testing.T) {
	t.Parallel()

	got, warnings := ResolveAlertConfig(staticLookup(map[string]string{
		"alert_task_blocked_too_long": "1h",
		"alert_task_stale":            "2h",
		"alert_review_too_long":       "3h",
		"alert_backlog_too_large":     "0",
	}, "global"))
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	want := map[AlertType]AlertThreshold{
		AlertTaskBlockedTooLong: {Type: AlertTaskBlockedTooLong, Severity: AlertSeverityHigh, Duration: time.Hour},
		AlertTaskStale:          {Type: AlertTaskStale, Severity: AlertSeverityMedium, Duration: 2 * time.Hour},
		AlertReviewTooLong:      {Type: AlertReviewTooLong, Severity: AlertSeverityMedium, Duration: 3 * time.Hour},
		// 0 is a legitimate count: "alert on any backlog at all".
		AlertBacklogTooLarge: {Type: AlertBacklogTooLarge, Severity: AlertSeverityLow, Count: 0},
	}
	for typ, wantTh := range want {
		th := got.GetThreshold(typ)
		if th == nil || *th != wantTh {
			t.Errorf("%s = %+v, want %+v", typ, th, wantTh)
		}
	}
}

// TestResolveAlertConfig_MalformedKeepsDefaultAndWarns is the non-fatal contract.
// Config load happens at App init, so a rejected value must degrade to the
// default with a warning — never kill the command.
func TestResolveAlertConfig_MalformedKeepsDefaultAndWarns(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		key       string
		alertType AlertType
		value     string
		wantInMsg string
	}{
		{name: "unparseable duration", key: "alert_task_stale", alertType: AlertTaskStale, value: "3 weeks", wantInMsg: "duration"},
		{name: "duration with no unit", key: "alert_task_stale", alertType: AlertTaskStale, value: "3", wantInMsg: "duration"},
		{name: "zero duration", key: "alert_review_too_long", alertType: AlertReviewTooLong, value: "0h", wantInMsg: "positive"},
		{name: "negative duration", key: "alert_task_blocked_too_long", alertType: AlertTaskBlockedTooLong, value: "-2d", wantInMsg: "positive"},
		{name: "count given as a duration", key: "alert_backlog_too_large", alertType: AlertBacklogTooLarge, value: "10d", wantInMsg: "whole number"},
		{name: "negative count", key: "alert_backlog_too_large", alertType: AlertBacklogTooLarge, value: "-1", wantInMsg: "negative"},
		{name: "fractional count", key: "alert_backlog_too_large", alertType: AlertBacklogTooLarge, value: "2.5", wantInMsg: "whole number"},
		{name: "duration given as a count", key: "alert_task_stale", alertType: AlertTaskStale, value: "72", wantInMsg: "duration"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, warnings := ResolveAlertConfig(staticLookup(map[string]string{tc.key: tc.value}, "repo"))

			wantTh := *DefaultAlertConfig().GetThreshold(tc.alertType)
			if th := got.GetThreshold(tc.alertType); th == nil || *th != wantTh {
				t.Errorf("malformed %s=%q changed the threshold: got %+v, want the default %+v",
					tc.key, tc.value, th, wantTh)
			}
			if len(warnings) != 1 {
				t.Fatalf("malformed %s=%q produced %d warnings, want exactly 1: %v",
					tc.key, tc.value, len(warnings), warnings)
			}
			msg := warnings[0].String()
			for _, want := range []string{tc.key, tc.value, "repo", tc.wantInMsg} {
				if !strings.Contains(msg, want) {
					t.Errorf("warning %q does not mention %q", msg, want)
				}
			}
		})
	}
}

// TestResolveAlertConfig_BlankIsUnset — a key present with an empty value means
// "I configured nothing", the same reading ParseInstructionPointers gives it. It
// keeps the default and is NOT worth a warning.
func TestResolveAlertConfig_BlankIsUnset(t *testing.T) {
	t.Parallel()

	got, warnings := ResolveAlertConfig(staticLookup(map[string]string{
		"alert_task_stale":        "",
		"alert_backlog_too_large": "   ",
	}, "repo"))
	if len(warnings) != 0 {
		t.Errorf("blank values produced warnings: %v", warnings)
	}
	if !reflect.DeepEqual(got, DefaultAlertConfig()) {
		t.Fatalf("blank values changed the config: %+v", got)
	}
}

// TestResolveAlertConfig_SurroundingWhitespaceTolerated — a YAML author's stray
// space must not cost them the setting.
func TestResolveAlertConfig_SurroundingWhitespaceTolerated(t *testing.T) {
	t.Parallel()

	got, warnings := ResolveAlertConfig(staticLookup(map[string]string{
		"alert_task_stale":        " 6h ",
		"alert_backlog_too_large": " 25 ",
	}, "repo"))
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if th := got.GetThreshold(AlertTaskStale); th.Duration != 6*time.Hour {
		t.Errorf("task_stale = %v, want 6h", th.Duration)
	}
	if th := got.GetThreshold(AlertBacklogTooLarge); th.Count != 25 {
		t.Errorf("backlog_too_large = %d, want 25", th.Count)
	}
}

// TestResolveAlertConfig_TierIsReported — the warning names the tier the bad
// value came from, because that is the file the reader has to go edit.
func TestResolveAlertConfig_TierIsReported(t *testing.T) {
	t.Parallel()

	for _, tier := range []string{"repo", "org", "global"} {
		t.Run(tier, func(t *testing.T) {
			t.Parallel()
			_, warnings := ResolveAlertConfig(staticLookup(map[string]string{"alert_task_stale": "nope"}, tier))
			if len(warnings) != 1 {
				t.Fatalf("want 1 warning, got %v", warnings)
			}
			if warnings[0].Tier != tier {
				t.Errorf("warning tier = %q, want %q", warnings[0].Tier, tier)
			}
			if !strings.Contains(warnings[0].String(), tier) {
				t.Errorf("warning %q does not name the %q tier", warnings[0].String(), tier)
			}
		})
	}
}

// TestWriteAlertConfigWarnings — warnings go to the writer the caller supplies
// (os.Stderr at the App seam), one line each, prefixed like every other
// non-fatal adb warning. No warnings must write nothing at all.
func TestWriteAlertConfigWarnings(t *testing.T) {
	t.Parallel()

	var empty bytes.Buffer
	WriteAlertConfigWarnings(&empty, nil)
	if empty.Len() != 0 {
		t.Errorf("no warnings wrote %q, want nothing", empty.String())
	}

	_, warnings := ResolveAlertConfig(staticLookup(map[string]string{
		"alert_task_stale":        "nope",
		"alert_backlog_too_large": "also-nope",
	}, "repo"))
	if len(warnings) != 2 {
		t.Fatalf("want 2 warnings, got %v", warnings)
	}

	var buf bytes.Buffer
	WriteAlertConfigWarnings(&buf, warnings)
	out := buf.String()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("wrote %d lines, want 2:\n%s", len(lines), out)
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "Warning: ") {
			t.Errorf("line %q does not start with the house warning prefix", line)
		}
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("output %q is not newline-terminated", out)
	}
}

// TestNewAlertEvaluator_ConfigAccessor — the evaluator must expose the config it
// evaluates against, so a caller (and a test at the App seam) can read the
// resolved thresholds without reaching into the struct.
func TestNewAlertEvaluator_ConfigAccessor(t *testing.T) {
	t.Parallel()

	if got := NewAlertEvaluator(nil, nil).Config(); !reflect.DeepEqual(got, DefaultAlertConfig()) {
		t.Fatalf("nil config: Config() = %+v, want the defaults", got)
	}

	custom := DefaultAlertConfig()
	custom.SetThreshold(AlertThreshold{Type: AlertTaskStale, Severity: AlertSeverityMedium, Duration: time.Hour})
	if got := NewAlertEvaluator(custom, nil).Config().GetThreshold(AlertTaskStale); got.Duration != time.Hour {
		t.Fatalf("Config() did not return the injected config: %+v", got)
	}

	var nilEvaluator *AlertEvaluator
	if got := nilEvaluator.Config(); got != nil {
		t.Fatalf("nil evaluator: Config() = %+v, want nil", got)
	}
}
