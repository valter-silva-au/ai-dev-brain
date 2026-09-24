package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const approvedEscapedRoleRemediation = "Preserve and review the role's existing content, then replace " +
	"the symbolic link with a real directory beneath the workspace."

type foundationFinding struct {
	ID          string   `json:"id"`
	Severity    string   `json:"severity"`
	Evidence    []string `json:"evidence"`
	Remediation string   `json:"remediation"`
}

type foundationResult struct {
	Capability string `json:"capability"`
	Version    string `json:"version"`
	Outcome    string `json:"outcome"`
	Data       struct {
		WorkspaceID string              `json:"workspace_id"`
		OperationID string              `json:"operation_id"`
		Root        string              `json:"root"`
		Findings    []foundationFinding `json:"findings"`
	} `json:"data"`
	Recovery struct {
		Required bool `json:"required"`
	} `json:"recovery"`
}

func TestE2E_V3FoundationLifecycle(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "AWS")

	planned := decodeFoundationResult(
		t,
		mustRunADB(
			t,
			parent,
			"init",
			root,
			"--name",
			"AWS",
			"--format",
			"json",
		).stdout,
	)
	if planned.Capability != "workspace.initialize" ||
		planned.Version != "v1" ||
		planned.Outcome != "planned" {
		t.Fatalf("unexpected initialization plan: %#v", planned)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run created workspace root: %v", err)
	}

	applied := decodeFoundationResult(
		t,
		mustRunADB(
			t,
			parent,
			"init",
			root,
			"--name",
			"AWS",
			"--apply",
			"--format",
			"json",
		).stdout,
	)
	if applied.Outcome != "applied" {
		t.Fatalf("apply outcome = %q, want applied", applied.Outcome)
	}
	if applied.Data.WorkspaceID == "" || applied.Data.OperationID == "" {
		t.Fatalf("apply omitted stable identifiers: %#v", applied.Data)
	}
	assertV3WorkspaceLayout(t, root)

	beforeRepeat := snapshotTreeHashes(t, root)
	repeated := decodeFoundationResult(
		t,
		mustRunADB(
			t,
			parent,
			"init",
			root,
			"--name",
			"AWS",
			"--apply",
			"--format",
			"json",
		).stdout,
	)
	if repeated.Outcome != "unchanged" {
		t.Fatalf("repeat outcome = %q, want unchanged", repeated.Outcome)
	}
	if repeated.Data.WorkspaceID != applied.Data.WorkspaceID {
		t.Fatalf(
			"repeat workspace id = %q, want %q",
			repeated.Data.WorkspaceID,
			applied.Data.WorkspaceID,
		)
	}
	afterRepeat := snapshotTreeHashes(t, root)
	if !reflect.DeepEqual(afterRepeat, beforeRepeat) {
		t.Fatalf(
			"repeated initialization mutated workspace\nbefore=%v\nafter=%v",
			beforeRepeat,
			afterRepeat,
		)
	}

	beforeDoctor := snapshotTreeHashes(t, root)
	healthy := decodeFoundationResult(
		t,
		mustRunADB(
			t,
			parent,
			"doctor",
			"--workspace",
			root,
			"--format",
			"json",
		).stdout,
	)
	if healthy.Capability != "workspace.doctor" ||
		healthy.Version != "v1" ||
		healthy.Outcome != "healthy" {
		t.Fatalf("unexpected doctor result: %#v", healthy)
	}
	afterDoctor := snapshotTreeHashes(t, root)
	if !reflect.DeepEqual(afterDoctor, beforeDoctor) {
		t.Fatalf(
			"doctor mutated healthy workspace\nbefore=%v\nafter=%v",
			beforeDoctor,
			afterDoctor,
		)
	}

	eventsRoot := filepath.Join(root, ".aidb", "events")
	sourceOperation := filepath.Join(eventsRoot, applied.Data.OperationID)
	corruptOperation := filepath.Join(eventsRoot, "corrupt-copy")
	copyDirectory(t, sourceOperation, corruptOperation)
	corruptFirstJournalEvent(t, corruptOperation)

	beforeCorruptDoctor := snapshotTreeHashes(t, root)
	attentionRun := runADB(
		t,
		parent,
		"doctor",
		"--workspace",
		root,
		"--format",
		"json",
	)
	if attentionRun.exitCode != 1 {
		t.Fatalf(
			"corrupt doctor exit = %d, want 1\nstdout:\n%s\nstderr:\n%s",
			attentionRun.exitCode,
			attentionRun.stdout,
			attentionRun.stderr,
		)
	}
	attention := decodeFoundationResult(
		t,
		attentionRun.stdout,
	)
	if attention.Outcome != "needs_attention" {
		t.Fatalf(
			"corrupt doctor outcome = %q, want needs_attention",
			attention.Outcome,
		)
	}
	assertFoundationFinding(
		t,
		attention,
		"operation.journal.invalid",
		"error",
	)
	afterCorruptDoctor := snapshotTreeHashes(t, root)
	if !reflect.DeepEqual(afterCorruptDoctor, beforeCorruptDoctor) {
		t.Fatalf(
			"doctor mutated corrupt workspace\nbefore=%v\nafter=%v",
			beforeCorruptDoctor,
			afterCorruptDoctor,
		)
	}
	if strings.Contains(attentionRun.combined(), "Usage:") {
		t.Fatalf("corrupt doctor printed usage:\n%s", attentionRun.combined())
	}
}

func TestE2E_V3DoctorReportsEscapedRoleWithExitOne(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "AWS")
	mustRunADB(
		t,
		parent,
		"init",
		root,
		"--name",
		"AWS",
		"--apply",
		"--format",
		"json",
	)

	external := filepath.Join(parent, "external-organizations")
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatalf("create external role target: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(external, "marker.txt"),
		[]byte("preserve me"),
		0o644,
	); err != nil {
		t.Fatalf("write external marker: %v", err)
	}
	organizations := filepath.Join(root, "organizations")
	if err := os.Remove(organizations); err != nil {
		t.Fatalf("remove initialized organizations directory: %v", err)
	}
	if err := os.Symlink(external, organizations); err != nil {
		t.Fatalf("redirect organizations role: %v", err)
	}

	beforeWorkspace := snapshotTreeHashes(t, root)
	beforeExternal := snapshotTreeHashes(t, external)
	run := runADB(
		t,
		parent,
		"doctor",
		"--workspace",
		root,
		"--format",
		"json",
	)
	if run.exitCode != 1 {
		t.Fatalf(
			"escaped doctor exit = %d, want 1\nstdout:\n%s\nstderr:\n%s",
			run.exitCode,
			run.stdout,
			run.stderr,
		)
	}
	result := decodeFoundationResult(t, run.stdout)
	if result.Outcome != "needs_attention" {
		t.Fatalf("escaped doctor outcome = %q, want needs_attention", result.Outcome)
	}
	finding := assertFoundationFinding(t, result, "workspace.role.escaped", "error")
	evidence := strings.Join(finding.Evidence, "\n")
	for _, expected := range []string{
		"role=organizations",
		"declared=organizations",
		"component=organizations",
	} {
		if !strings.Contains(evidence, expected) {
			t.Fatalf("escaped finding evidence missing %q: %#v", expected, finding.Evidence)
		}
	}
	if finding.Remediation != approvedEscapedRoleRemediation {
		t.Fatalf(
			"escaped remediation = %q, want %q",
			finding.Remediation,
			approvedEscapedRoleRemediation,
		)
	}
	if !result.Recovery.Required {
		t.Fatal("escaped doctor recovery.required = false, want true")
	}
	if strings.Contains(run.stdout, "Usage:") {
		t.Fatalf("escaped doctor printed usage on stdout:\n%s", run.stdout)
	}
	if run.stderr != "" {
		t.Fatalf("escaped doctor wrote stderr noise:\n%s", run.stderr)
	}
	linkTarget, err := os.Readlink(organizations)
	if err != nil {
		t.Fatalf("escaped role is no longer a symlink: %v", err)
	}
	if linkTarget != external {
		t.Fatalf("escaped role target = %q, want %q", linkTarget, external)
	}
	afterWorkspace := snapshotTreeHashes(t, root)
	if !reflect.DeepEqual(afterWorkspace, beforeWorkspace) {
		t.Fatalf(
			"doctor mutated escaped workspace\nbefore=%v\nafter=%v",
			beforeWorkspace,
			afterWorkspace,
		)
	}
	afterExternal := snapshotTreeHashes(t, external)
	if !reflect.DeepEqual(afterExternal, beforeExternal) {
		t.Fatalf(
			"doctor mutated escaped role target\nbefore=%v\nafter=%v",
			beforeExternal,
			afterExternal,
		)
	}
}

func decodeFoundationResult(t *testing.T, output string) foundationResult {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(output))
	var result foundationResult
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode foundation result: %v\n%s", err, output)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("foundation output contains more than one JSON value: %v", err)
	}
	return result
}

func assertV3WorkspaceLayout(t *testing.T, root string) {
	t.Helper()

	for _, path := range []string{
		filepath.Join(root, ".aidb", "manifest.yaml"),
		filepath.Join(root, ".aidb", "config.yaml"),
		filepath.Join(root, ".aidb", "state.sqlite"),
		filepath.Join(root, ".aidb", "events"),
		filepath.Join(root, ".aidb", "cache"),
		filepath.Join(root, "organizations"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected v3 workspace path %q: %v", path, err)
		}
	}
}

func assertFoundationFinding(
	t *testing.T,
	result foundationResult,
	id string,
	severity string,
) foundationFinding {
	t.Helper()

	for _, finding := range result.Data.Findings {
		if finding.ID == id && finding.Severity == severity {
			return finding
		}
	}
	t.Fatalf(
		"finding %q/%q not present in %#v",
		id,
		severity,
		result.Data.Findings,
	)
	return foundationFinding{}
}

func snapshotTreeHashes(t *testing.T, root string) map[string]string {
	t.Helper()

	snapshot := make(map[string]string)
	if err := filepath.WalkDir(
		root,
		func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				target, err := os.Readlink(path)
				if err != nil {
					return err
				}
				snapshot[relative] = "symlink->" + target
				return nil
			}
			if entry.IsDir() {
				snapshot[relative+"/"] = "directory"
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			digest := sha256.Sum256(content)
			snapshot[relative] = hex.EncodeToString(digest[:])
			return nil
		},
	); err != nil {
		t.Fatalf("snapshot workspace: %v", err)
	}
	return snapshot
}

func copyDirectory(t *testing.T, source string, destination string) {
	t.Helper()

	if err := filepath.WalkDir(
		source,
		func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			target := filepath.Join(destination, relative)
			if entry.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(target, content, 0o644)
		},
	); err != nil {
		t.Fatalf("copy journal operation: %v", err)
	}
}

func corruptFirstJournalEvent(t *testing.T, operationDir string) {
	t.Helper()

	entries, err := os.ReadDir(operationDir)
	if err != nil {
		t.Fatalf("read copied journal operation: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() ||
			entry.Name() == "plan.json" ||
			!strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if err := os.WriteFile(
			filepath.Join(operationDir, entry.Name()),
			[]byte("{not-json"),
			0o644,
		); err != nil {
			t.Fatalf("corrupt copied journal event: %v", err)
		}
		return
	}
	t.Fatal("copied journal operation has no event to corrupt")
}
