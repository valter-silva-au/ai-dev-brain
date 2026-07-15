package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/internal/integration"
	"github.com/valter-silva-au/ai-dev-brain/internal/scheduler"
	"github.com/valter-silva-au/ai-dev-brain/internal/statedir"
)

// NewSchedulerCmd creates the `adb scheduler` command group.
func NewSchedulerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scheduler",
		Short: "Run recurring maintenance jobs in the background",
		Long: `adb has a small built-in scheduler that runs recurring jobs:

  repos-pull     fetch + fast-forward every repo under <workspace>/repos
  alerts-tick    evaluate alert conditions and log transitions
  events-rotate  size-check the event and scheduler logs, rotate if large

Start:    adb scheduler start
Stop:     adb scheduler stop
List:     adb scheduler list
Status:   adb scheduler status

Or run foreground (what the detached daemon invokes):
          adb scheduler run`,
	}
	cmd.AddCommand(
		newSchedulerStartCmd(),
		newSchedulerStopCmd(),
		newSchedulerRestartCmd(),
		newSchedulerStatusCmd(),
		newSchedulerRunCmd(),
		newSchedulerListCmd(),
	)
	return cmd
}

// ---- paths ----

// Scheduler state lives under .adb/ (#186). schedulerBase() still resolves the
// workspace root (App.BasePath → ADB_HOME → cwd) so `adb scheduler status`/`stop`
// find a daemon after the migration; statedir.Path appends the .adb/ segment.
func schedulerPIDPath() string {
	return statedir.Path(schedulerBase(), statedir.FileSchedulerPID)
}

func schedulerLogPath() string {
	return statedir.Path(schedulerBase(), statedir.FileSchedulerLog)
}

func schedulerStatePath() string {
	return statedir.Path(schedulerBase(), statedir.FileSchedulerState)
}

func schedulerBase() string {
	if App != nil && App.BasePath != "" {
		return App.BasePath
	}
	if p := os.Getenv("ADB_HOME"); p != "" {
		return p
	}
	return "."
}

// ---- subcommands ----

func newSchedulerStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the scheduler as a background daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return schedulerDaemonStart()
		},
	}
}

func newSchedulerStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the scheduler daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return schedulerDaemonStop()
		},
	}
}

func newSchedulerRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart the scheduler daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = schedulerDaemonStop()
			return schedulerDaemonStart()
		},
	}
}

func newSchedulerStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Report whether the scheduler daemon is running",
		RunE: func(cmd *cobra.Command, args []string) error {
			pid, alive := readPIDFile(schedulerPIDPath())
			switch {
			case alive:
				fmt.Printf("✓ Scheduler running (PID %d)\n", pid)
				fmt.Printf("  log:   %s\n", schedulerLogPath())
				fmt.Printf("  state: %s\n", schedulerStatePath())
			case pid > 0:
				fmt.Printf("✗ Scheduler not running (stale PID %d)\n", pid)
				_ = os.Remove(schedulerPIDPath())
			default:
				fmt.Println("✗ Scheduler not running")
			}
			return nil
		},
	}
}

func newSchedulerRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run the scheduler in the foreground (used by the daemon)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return schedulerRunForeground()
		},
	}
}

func newSchedulerListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List scheduler jobs and their last-run state",
		RunE: func(cmd *cobra.Command, args []string) error {
			states, err := scheduler.LoadStates(schedulerStatePath())
			if err != nil {
				return fmt.Errorf("load state: %w", err)
			}
			byName := make(map[string]scheduler.State)
			for _, s := range states {
				byName[s.Name] = s
			}

			// Display the three built-in jobs (with their known default intervals)
			// AND every job that has persisted state — the daemon also runs one
			// `rule:<name>` job per enabled time rule plus `automation-dispatch`,
			// whose state is recorded the same way. Listing only DefaultJobs hid
			// those, so a failing rule job was invisible in `adb scheduler list`
			// (#178). Built-ins first (in order), then any extra stateful jobs
			// (sorted) that aren't built-ins.
			intervals := make(map[string]string)
			var order []string
			seen := make(map[string]bool)
			for _, j := range scheduler.DefaultJobs(scheduler.Deps{}) {
				intervals[j.Name] = j.DefaultInterval.String()
				order = append(order, j.Name)
				seen[j.Name] = true
			}
			extra := make([]string, 0, len(states))
			for _, s := range states {
				if !seen[s.Name] {
					extra = append(extra, s.Name)
					seen[s.Name] = true
				}
			}
			sort.Strings(extra)
			order = append(order, extra...)

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "JOB\tINTERVAL\tRUNS\tFAILURES\tSKIPPED\tLAST_RUN\tLAST_DURATION\tLAST_ERROR")
			for _, name := range order {
				s := byName[name]
				interval := intervals[name]
				if interval == "" {
					// A rule:*/automation-dispatch job — its interval lives in the
					// rule/automation config, not knowable from state alone.
					interval = "-"
				}
				lastRun := "-"
				if !s.LastStart.IsZero() {
					lastRun = s.LastStart.Local().Format(time.RFC3339)
				}
				fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%s\t%s\t%s\n",
					name,
					interval,
					s.Runs,
					s.Failures,
					s.Skipped,
					lastRun,
					s.LastDuration,
					truncateText(s.LastError, 60),
				)
			}
			return w.Flush()
		},
	}
}

func truncateText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// ---- daemon lifecycle (mirrors serve.go) ----

func schedulerDaemonStart() error {
	if pid, alive := readPIDFile(schedulerPIDPath()); alive {
		return fmt.Errorf("scheduler already running (PID %d). Use 'adb scheduler restart'", pid)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find adb binary: %w", err)
	}

	cmd := exec.Command(exe, "scheduler", "run")
	cmd.Env = os.Environ()
	cmd.Stdout = nil
	cmd.Stderr = nil
	detachProcess(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start scheduler daemon: %w", err)
	}
	pid := cmd.Process.Pid
	if err := os.WriteFile(schedulerPIDPath(), []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return fmt.Errorf("write PID file: %w", err)
	}
	_ = cmd.Process.Release()

	fmt.Printf("✓ Scheduler started (PID %d)\n", pid)
	fmt.Printf("  log:   %s\n", schedulerLogPath())
	fmt.Printf("  stop:  adb scheduler stop\n")
	return nil
}

func schedulerDaemonStop() error {
	pid, alive := readPIDFile(schedulerPIDPath())
	if !alive {
		if pid > 0 {
			_ = os.Remove(schedulerPIDPath())
		}
		fmt.Println("Scheduler is not running.")
		return nil
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		_ = os.Remove(schedulerPIDPath())
		return nil
	}
	if err := stopProcess(p); err != nil {
		_ = os.Remove(schedulerPIDPath())
		fmt.Printf("✓ Scheduler stopped (PID %d was not running)\n", pid)
		return nil
	}
	_ = os.Remove(schedulerPIDPath())
	fmt.Printf("✓ Scheduler stopped (PID %d)\n", pid)
	return nil
}

// readPIDFile reads a PID file and checks whether the process is alive.
// Extracted so serve.go and scheduler.go don't need to share a private
// helper.
func readPIDFile(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return pid, false
	}
	return pid, processAlive(p)
}

// ---- foreground loop ----

func schedulerRunForeground() error {
	if App == nil {
		return fmt.Errorf("app not initialized")
	}

	// Open (or create) the log file and duplicate output to it.
	if err := statedir.Ensure(schedulerBase()); err != nil {
		return fmt.Errorf("prep log dir: %w", err)
	}
	logFile, err := os.OpenFile(schedulerLogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer logFile.Close()

	logger := io.MultiWriter(os.Stdout, logFile)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps := scheduler.Deps{
		BasePath: App.BasePath,
		Logger:   logger,
		PullRepos: func(ctx context.Context) (string, error) {
			summary, err := integration.PullAllRepos(App.BasePath, integration.PullOpts{})
			if err != nil {
				return "", err
			}
			return summary.Format(), nil
		},
		EvaluateAlerts: func(ctx context.Context) (int, string, error) {
			if App.AlertEvaluator == nil {
				return 0, "", nil
			}
			alerts, err := App.AlertEvaluator.EvaluateAll()
			if err != nil {
				return 0, "", err
			}
			var buf bytes.Buffer
			for _, a := range alerts {
				fmt.Fprintf(&buf, "      [%s] %s\n", a.Severity, a.Message)
			}
			return len(alerts), buf.String(), nil
		},
		LogFiles: []string{
			statedir.Path(App.BasePath, statedir.FileEventsLog),
			schedulerLogPath(),
		},
	}

	jobs := scheduler.DefaultJobs(deps)

	// Generalize the scheduler with the declarative rule engine (decision D7):
	// every enabled time-triggered rule becomes a recurring job, and — when
	// automation is opted in — an automation-dispatch job drains new events and
	// fires matching event-triggered rules.
	jobs = append(jobs, ruleJobs(logger)...)
	if automationEnabled() {
		jobs = append(jobs, automationDispatchJob(logger))
	}

	fmt.Fprintf(logger, "adb scheduler starting with %d jobs\n", len(jobs))
	return scheduler.Run(ctx, scheduler.RunOptions{
		Jobs:       jobs,
		StateFile:  schedulerStatePath(),
		Logger:     logger,
		RunOnStart: false, // avoid a pull storm at daemon startup
	})
}

// automationEnabled reports whether event-triggered dispatch is opted in via
// the merged config (automation.enabled). Defaults to false.
func automationEnabled() bool {
	if App == nil || App.MergedConfig == nil || App.MergedConfig.Global == nil {
		return false
	}
	return App.MergedConfig.Global.Automation.Enabled
}

// automationDispatchInterval is the drain cadence for the automation-dispatch
// job. Reads automation.dispatch_interval (a Go duration); defaults to 30s.
func automationDispatchInterval() time.Duration {
	const fallback = 30 * time.Second
	if App == nil || App.MergedConfig == nil || App.MergedConfig.Global == nil {
		return fallback
	}
	raw := strings.TrimSpace(App.MergedConfig.Global.Automation.DispatchInterval)
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

// ruleJobs turns each enabled time-triggered rule into a scheduler job whose Run
// fires the rule. A firing that errors fails the job (so `adb scheduler list`
// surfaces it); a skipped/fired firing is logged and the job succeeds.
func ruleJobs(logger io.Writer) []scheduler.Job {
	if App == nil || App.RuleEngine == nil {
		return nil
	}
	rules, err := App.RuleEngine.TimeRules()
	if err != nil {
		fmt.Fprintf(logger, "    automation: load time rules: %v\n", err)
		return nil
	}
	jobs := make([]scheduler.Job, 0, len(rules))
	for _, r := range rules {
		interval, err := r.On.Interval()
		if err != nil {
			fmt.Fprintf(logger, "    automation: rule %q has bad schedule: %v\n", r.Name, err)
			continue
		}
		name := r.Name
		jobs = append(jobs, scheduler.Job{
			Name:            "rule:" + name,
			DefaultInterval: interval,
			Run: func(ctx context.Context) error {
				f, err := App.RuleEngine.FireByName(ctx, name, nil)
				if err != nil {
					return err
				}
				fmt.Fprintf(logger, "    rule %s: %s %s\n", f.Rule, f.Status, firingDetail(f))
				if f.Status == core.FiringError {
					return fmt.Errorf("rule %s errored: %s", f.Rule, f.Reason)
				}
				return nil
			},
		})
	}
	return jobs
}

// automationDispatchJob drains new .events.jsonl entries past a persisted cursor
// and fires matching event-triggered rules. On first run (no cursor) it seeds
// the cursor to "now" so historical events are not replayed.
func automationDispatchJob(logger io.Writer) scheduler.Job {
	return scheduler.Job{
		Name:            "automation-dispatch",
		DefaultInterval: automationDispatchInterval(),
		Run: func(ctx context.Context) error {
			return drainAutomationEvents(ctx, logger)
		},
	}
}

func automationCursorPath() string {
	return statedir.Path(schedulerBase(), statedir.FileAutomationCursor)
}

func readAutomationCursor() (time.Time, bool) {
	data, err := os.ReadFile(automationCursorPath())
	if err != nil {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(data)))
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func writeAutomationCursor(t time.Time) error {
	return os.WriteFile(automationCursorPath(), []byte(t.UTC().Format(time.RFC3339Nano)), 0o644)
}

// drainAutomationEvents reads events after the cursor and dispatches each to the
// rule engine, advancing the cursor to the newest processed event. Fired/errored
// firings are logged.
//
// Semantics are FIRE-AND-FORGET: the cursor advances past every event in the
// batch even when a firing errors, so one persistently-failing rule can never
// wedge the whole dispatch. The trade-off is no retry — a transient action
// failure (e.g. a flaky exec) is logged and skipped, not replayed. A rule that
// must not miss an event should be idempotent and reconcile from state.
func drainAutomationEvents(ctx context.Context, logger io.Writer) error {
	if App == nil || App.RuleEngine == nil || App.EventLog == nil {
		return nil
	}
	cursor, ok := readAutomationCursor()
	if !ok {
		// First drain: seed to now so we react only to events from here on.
		now := time.Now().UTC()
		if err := writeAutomationCursor(now); err != nil {
			return fmt.Errorf("seed automation cursor: %w", err)
		}
		return nil
	}
	events, err := App.EventLog.ReadSince(cursor)
	if err != nil {
		return fmt.Errorf("read events since cursor: %w", err)
	}
	newCursor := cursor
	for _, ev := range events {
		if !ev.Timestamp.After(cursor) {
			continue // boundary event already processed
		}
		firings, derr := App.RuleEngine.Dispatch(ctx, string(ev.Type), eventPayload(ev.Data))
		if derr != nil {
			fmt.Fprintf(logger, "    automation-dispatch: %v\n", derr)
		}
		for _, f := range firings {
			fmt.Fprintf(logger, "    dispatch %s [%s]: %s %s\n", ev.Type, f.Rule, f.Status, firingDetail(f))
		}
		if ev.Timestamp.After(newCursor) {
			newCursor = ev.Timestamp
		}
	}
	if newCursor.After(cursor) {
		if err := writeAutomationCursor(newCursor); err != nil {
			return fmt.Errorf("advance automation cursor: %w", err)
		}
	}
	return nil
}

// eventPayload flattens an event's Data map into string values for template
// expansion.
func eventPayload(data map[string]interface{}) map[string]string {
	if len(data) == 0 {
		return nil
	}
	out := make(map[string]string, len(data))
	for k, v := range data {
		out[k] = stringifyEventValue(v)
	}
	return out
}

// stringifyEventValue renders one event-payload value for template use. JSON
// numbers arrive as float64; rendering them with strconv 'f' avoids scientific
// notation and prints whole numbers without a trailing ".0", so a numeric id
// substitutes cleanly (float64 precision limits on huge integers are inherent to
// the JSON event log, not introduced here).
func stringifyEventValue(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// firingDetail returns the human detail for a firing: its output when fired,
// else its reason.
func firingDetail(f core.Firing) string {
	if f.Status == core.FiringFired {
		return f.Output
	}
	return f.Reason
}
