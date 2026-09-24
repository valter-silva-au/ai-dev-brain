package issuesync

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// Direction controls which sides a sync may write.
type Direction string

const (
	DirectionBoth Direction = "both"
	DirectionPush Direction = "push" // local -> remote only
	DirectionPull Direction = "pull" // remote -> local only
)

// Action is the reconcile verdict — the pure output of Reconcile before any
// I/O.
type Action string

const (
	ActionNoop         Action = "noop"
	ActionCreateRemote Action = "create_remote"
	ActionUpdateRemote Action = "update_remote"
	ActionUpdateLocal  Action = "update_local"
)

// Input bundles everything Reconcile needs. It is deliberately I/O-free so
// the reconcile engine is pure and table-testable with no network.
type Input struct {
	Local        models.Task
	Body         string
	Remote       RemoteIssue
	RemoteFound  bool
	Baseline     string    // stored Task.SyncHash from the last sync ("" = never synced)
	LocalUpdated time.Time // Task.Updated
	Direction    Direction
}

// Decision is the reconcile result plus a human/loggable reason.
type Decision struct {
	Action Action
	Reason string
}

func (d Direction) canPush() bool { return d == DirectionBoth || d == DirectionPush }
func (d Direction) canPull() bool { return d == DirectionBoth || d == DirectionPull }

// SyncHash is the reconcile baseline: a stable digest of ONLY the synced
// fields (title, body, labels-derived-from-status+priority, status,
// priority). Everything else on a Task (Owner, Tags outside labels,
// timestamps, paths, remote linkage) is out of scope and never overwritten.
//
// body is passed separately because adb has no body field on Task — it lives
// in the ticket's context.md — so the caller supplies it.
//
// Reconcile compares SyncHash(local, body) against Task.SyncHash to detect a
// LOCAL edit since the last sync. This is the fold-in-#3 fix: baseline is a
// stored per-sync value, not the LOCAL Updated timestamp (which would be
// unsound because a `save` bumps Updated whether or not any synced field
// changed).
func SyncHash(t models.Task, body string) string {
	labels := []string{StatusLabel(t.Status), PriorityLabel(t.Priority)}
	sort.Strings(labels)
	// \x00 is a byte no synced field can contain — keeps the concat unambiguous.
	payload := strings.Join([]string{
		t.Title,
		body,
		strings.Join(labels, ","),
		string(t.Status),
		string(t.Priority),
	}, "\x00")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// Reconcile applies last-writer-wins over the synced-fields allowlist. It
// never performs I/O; all shell-outs happen in the caller (Syncer).
//
// Change-detection semantics:
//   - localChanged = SyncHash(Local, Body) differs from the stored Baseline
//     (empty Baseline → never synced → always treated as changed so a first
//     sync pushes).
//   - remoteChanged is a HEURISTIC: RemoteIssue.UpdatedAt.After(LocalUpdated).
//     Because we don't store a separate remote-hash baseline, an updated_at
//     bump caused by a maintainer's non-synced change (e.g. edit body but
//     revert) can still trigger a pull. Documented, tested LWW-by-timestamp
//     limitation until a follow-up adds a remote-baseline field.
//
// Both changed → newer timestamp wins; --direction can veto either write.
func Reconcile(in Input) Decision {
	if !in.RemoteFound || in.Remote.Number == 0 {
		if in.Direction.canPush() {
			return Decision{ActionCreateRemote, "no remote issue; creating from local"}
		}
		return Decision{ActionNoop, "no remote issue and push disabled"}
	}

	localChanged := in.Baseline == "" || SyncHash(in.Local, in.Body) != in.Baseline
	remoteChanged := in.Remote.UpdatedAt.After(in.LocalUpdated)

	switch {
	case localChanged && remoteChanged:
		// LWW: whichever side has the later timestamp wins. `remoteChanged`
		// already implies Remote.UpdatedAt > LocalUpdated, so the tie-break
		// is really "did we get here via a non-monotonic clock or an equal
		// timestamp?" — extra guard kept for readability.
		if in.Remote.UpdatedAt.After(in.LocalUpdated) {
			if in.Direction.canPull() {
				return Decision{ActionUpdateLocal, "both changed; remote newer -> pull"}
			}
			return Decision{ActionNoop, "both changed; remote newer but pull disabled"}
		}
		if in.Direction.canPush() {
			return Decision{ActionUpdateRemote, "both changed; local newer -> push"}
		}
		return Decision{ActionNoop, "both changed; local newer but push disabled"}
	case localChanged:
		if in.Direction.canPush() {
			return Decision{ActionUpdateRemote, "local changed -> push"}
		}
		return Decision{ActionNoop, "local changed but push disabled"}
	case remoteChanged:
		if in.Direction.canPull() {
			return Decision{ActionUpdateLocal, "remote changed -> pull"}
		}
		return Decision{ActionNoop, "remote changed but pull disabled"}
	default:
		return Decision{ActionNoop, "in sync"}
	}
}
