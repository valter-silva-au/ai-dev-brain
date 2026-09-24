package observability

// Alert thresholds from the layered config.
//
// `adb alerts` evaluates four conditions against the thresholds in AlertConfig.
// Those thresholds used to be unreachable: the App seam constructed the evaluator
// with a nil config, NewAlertEvaluator fell back to DefaultAlertConfig()
// unconditionally, and AlertConfig.SetThreshold — a working setter — had no
// caller. This file is the missing reader.
//
// THE SHAPE, and why. Each threshold is ONE flat custom_settings key,
// "alert_" + the AlertType string:
//
//	custom_settings:
//	  alert_task_blocked_too_long: 48h
//	  alert_task_stale: 7d
//	  alert_review_too_long: 3d
//	  alert_backlog_too_large: 25
//
// Three properties fall out of that choice, and each is why it beats one
// structured value (e.g. `alert_thresholds: "task_stale=7d,..."`):
//
//   - custom_settings is declared as a flat map[string]string. Four string keys
//     fit that type exactly; a nested or list-shaped value fights it, and a value
//     that cannot be a string is skipped by core's normalizeCustomSettings with a
//     warning — so a structured spelling would have to be its own mini-grammar
//     inside a string, with its own parser and its own failure modes.
//   - Per-key TIER PRECEDENCE comes for free. A repo .taskrc can override one
//     threshold while the other three keep resolving from the org or global tier.
//     A single packed value can only be overridden wholesale: a repo that wanted a
//     different backlog size would have to restate the other three.
//   - `adb config get alert_task_stale --source` already works, and names the tier
//     that won. A packed value can only report the tier of the whole blob.
//
// Keys are UNDERSCORED. Viper treats "." as a key-nesting delimiter, so a dotted
// `alert.task_stale` decodes as a nested map where a string is expected — the trap
// documented at length in L600 §11 and worked around by normalizeCustomSettings.
// Underscores sidestep it, matching programs_search_paths.
//
// SEVERITY IS NOT CONFIGURABLE. It stays whatever DefaultAlertConfig() says. A
// severity is a fixed three-value triage label with no threshold semantics, and
// making it configurable would double the key count for no operational gain — the
// question users actually have is "when does this fire?", not "what colour is it?".
//
// NOTHING HERE IS FATAL. Config resolution happens at App init, so a rejected
// value that returned an error would kill EVERY command in the workspace — the
// exact defect L600 §11 records for a dotted custom-settings key. A value that
// cannot be parsed is therefore skipped with a warning on stderr (the caller's
// writer) and the default is kept, so the worst a typo costs you is one threshold.

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// alertSettingPrefix namespaces the alert threshold keys inside the shared,
// flat custom_settings map.
const alertSettingPrefix = "alert_"

// AlertSettingKey returns the custom_settings key that configures alertType's
// threshold. The key is derived from the AlertType rather than written out
// separately, so the config vocabulary can never drift from the alert vocabulary.
func AlertSettingKey(alertType AlertType) string {
	return alertSettingPrefix + string(alertType)
}

// AlertSettingKeys returns every configurable threshold key, in
// DefaultAlertConfig() order. Adding a default threshold therefore makes it
// configurable automatically — a new condition cannot ship unreachable the way
// these four did.
func AlertSettingKeys() []string {
	defaults := DefaultAlertConfig().Thresholds
	keys := make([]string, 0, len(defaults))
	for _, th := range defaults {
		keys = append(keys, AlertSettingKey(th.Type))
	}
	return keys
}

// countAlertTypes lists the thresholds whose value is a COUNT rather than a
// duration. Membership is declared, not inferred from a default's zero fields:
// "alert on any backlog at all" is a legitimate count of 0, and inferring the
// shape from the value would silently reclassify such a threshold as time-based.
var countAlertTypes = map[AlertType]bool{
	AlertBacklogTooLarge: true,
}

// AlertSettingLookup resolves one custom-settings key to its value and the tier
// that supplied it. It is exactly the signature of
// models.MergedConfig.SettingSource, passed as a func so this package stays free
// of a models import (and so a test can supply a fixture without a config file).
type AlertSettingLookup func(key string) (value, tier string, ok bool)

// AlertConfigWarning records one rejected threshold value. It carries the key,
// the tier that supplied it and the offending value, because the reader's next
// action is to go edit that specific file.
// Err leads the struct only to satisfy govet's fieldalignment; the fields read
// Key/Tier/Value/Err in String() and in every caller.
type AlertConfigWarning struct {
	Err   error
	Key   string
	Tier  string
	Value string
}

// String renders the warning as one line, naming the key, the tier, the bad
// value, why it was rejected, and the default that was kept instead.
func (w AlertConfigWarning) String() string {
	tier := w.Tier
	if tier == "" {
		tier = "config"
	}
	return fmt.Sprintf("ignoring alert threshold %q from the %s tier: %v", w.Key, tier, w.Err)
}

// ResolveAlertConfig builds the alert configuration from the layered config,
// starting from DefaultAlertConfig() and overriding only the thresholds a tier
// actually sets. A nil lookup, an absent key, or a blank value all leave the
// default in place, so an unconfigured workspace resolves to exactly the
// defaults.
//
// It is pure: warnings are returned rather than printed, so the caller decides
// the channel (WriteAlertConfigWarnings writes them to stderr at the App seam)
// and a test can assert them without capturing a stream.
func ResolveAlertConfig(lookup AlertSettingLookup) (*AlertConfig, []AlertConfigWarning) {
	config := DefaultAlertConfig()
	if lookup == nil {
		return config, nil
	}

	var warnings []AlertConfigWarning
	// Iterate the DEFAULTS, not the configured keys: that is what guarantees a
	// threshold with no config entry keeps its default byte-for-byte, and what
	// makes a newly-added default configurable with no change here.
	for _, def := range DefaultAlertConfig().Thresholds {
		key := AlertSettingKey(def.Type)
		raw, tier, ok := lookup(key)
		if !ok {
			continue
		}
		value := strings.TrimSpace(raw)
		if value == "" {
			// A key set to nothing means "I configured nothing" — the same reading
			// core.ParseInstructionPointers gives a blank value. Not worth a warning.
			continue
		}

		threshold := def // severity and type come from the default; only the bar moves
		var err error
		if countAlertTypes[def.Type] {
			threshold.Count, err = parseAlertCount(value)
		} else {
			threshold.Duration, err = parseAlertDuration(value)
		}
		if err != nil {
			warnings = append(warnings, AlertConfigWarning{Key: key, Tier: tier, Value: raw, Err: err})
			continue
		}
		config.SetThreshold(threshold)
	}

	return config, warnings
}

// parseAlertDuration parses a time-based threshold. It reuses ParseDuration, so
// "3d" means three days here exactly as it does for `adb metrics --since`.
//
// A non-positive duration is rejected: the evaluators fire on
// `elapsed > threshold`, so 0 would alert on every task in the status the moment
// it entered it — a value nobody means to write, and one whose effect is loud
// enough to look like a bug in `adb alerts` rather than a bad config line.
func parseAlertDuration(value string) (time.Duration, error) {
	d, err := ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%q is not a duration (want e.g. 24h, 3d, 90m); keeping the default", value)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%q must be a positive duration; keeping the default", value)
	}
	return d, nil
}

// parseAlertCount parses a count-based threshold. Zero is allowed — it means
// "alert as soon as there is anything at all" — but a negative count is not,
// since `size > threshold` would then alert on an empty backlog.
func parseAlertCount(value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%q is not a whole number of tasks; keeping the default", value)
	}
	if n < 0 {
		return 0, fmt.Errorf("%q must not be negative; keeping the default", value)
	}
	return n, nil
}

// WriteAlertConfigWarnings writes each warning as one prefixed line to w. The
// caller passes os.Stderr: a config diagnostic must never land on stdout, where
// it would corrupt a `--json` pipeline.
func WriteAlertConfigWarnings(w io.Writer, warnings []AlertConfigWarning) {
	if w == nil {
		return
	}
	for _, warning := range warnings {
		fmt.Fprintf(w, "Warning: %s\n", warning)
	}
}
