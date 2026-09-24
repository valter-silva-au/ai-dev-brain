package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type repositoryView struct {
	ID            string `json:"id"`
	Host          string `json:"host"`
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	Path          string `json:"path"`
	ClonePath     string `json:"clone_path"`
	ExternalClone bool   `json:"external_clone"`
}

type repositoryResult struct {
	Capability string `json:"capability"`
	Version    string `json:"version"`
	Outcome    string `json:"outcome"`
	Data       struct {
		Repository   repositoryView   `json:"repository"`
		Repositories []repositoryView `json:"repositories"`
		OperationID  string           `json:"operation_id"`
		State        string           `json:"state"`
	} `json:"data"`
}

func TestE2E_V3RepositoryInitializeAdoptAndFastForward(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "AWS")
	initializeV3Workspace(t, parent, root)
	initializeV3Organization(t, parent, root, "amazon", "Amazon")

	remoteIdentity := "https://github.com/example/managed.git"
	planned := decodeSingleJSON[repositoryResult](
		t,
		mustRunADB(
			t,
			parent,
			"repo",
			"add",
			remoteIdentity,
			"--workspace",
			root,
			"--org",
			"amazon",
			"--mode",
			"initialize-local",
			"--format",
			"json",
		).stdout,
	)
	if planned.Capability != "repository.add" ||
		planned.Version != "v1" ||
		planned.Outcome != "planned" {
		t.Fatalf("unexpected repository plan: %#v", planned)
	}
	if _, err := os.Stat(planned.Data.Repository.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("repository plan created %q: %v", planned.Data.Repository.Path, err)
	}

	applied := decodeSingleJSON[repositoryResult](
		t,
		mustRunADB(
			t,
			parent,
			"repo",
			"add",
			remoteIdentity,
			"--workspace",
			root,
			"--org",
			"amazon",
			"--mode",
			"initialize-local",
			"--apply",
			"--format",
			"json",
		).stdout,
	)
	if applied.Outcome != "applied" ||
		applied.Data.Repository.ID == "" ||
		applied.Data.OperationID == "" ||
		applied.Data.Repository.ExternalClone {
		t.Fatalf("unexpected repository apply: %#v", applied)
	}
	repositoryID := applied.Data.Repository.ID
	clonePath := applied.Data.Repository.ClonePath

	beforeReads := snapshotTreeHashes(t, root)
	listed := decodeSingleJSON[repositoryResult](
		t,
		mustRunADB(
			t,
			parent,
			"repo",
			"list",
			"--workspace",
			root,
			"--org",
			"amazon",
			"--format",
			"json",
		).stdout,
	)
	shown := decodeSingleJSON[repositoryResult](
		t,
		mustRunADB(
			t,
			parent,
			"repo",
			"show",
			repositoryID,
			"--workspace",
			root,
			"--org",
			"amazon",
			"--format",
			"json",
		).stdout,
	)
	afterReads := snapshotTreeHashes(t, root)
	if !reflect.DeepEqual(beforeReads, afterReads) {
		t.Fatalf(
			"repository list/show mutated workspace\nbefore=%v\nafter=%v",
			beforeReads,
			afterReads,
		)
	}
	if len(listed.Data.Repositories) != 1 ||
		listed.Data.Repositories[0].ID != repositoryID ||
		shown.Data.Repository.ID != repositoryID {
		t.Fatalf("repository reads lost identity: list=%#v show=%#v", listed, shown)
	}

	bareRemote := filepath.Join(parent, "managed.git")
	mustRunGit(t, parent, "init", "--bare", bareRemote)
	commitFile(t, clonePath, "README.md", "initial\n", "initial")
	mustRunGit(t, clonePath, "branch", "-M", "main")
	mustRunGit(t, clonePath, "remote", "set-url", "origin", bareRemote)
	mustRunGit(t, clonePath, "push", "-u", "origin", "main")
	mustRunGit(t, bareRemote, "symbolic-ref", "HEAD", "refs/heads/main")
	mustRunGit(t, clonePath, "remote", "set-head", "origin", "main")

	healthy := decodeSingleJSON[repositoryResult](
		t,
		mustRunADB(
			t,
			parent,
			"repo",
			"health",
			repositoryID,
			"--workspace",
			root,
			"--org",
			"amazon",
			"--format",
			"json",
		).stdout,
	)
	if healthy.Outcome != "healthy" || healthy.Data.State != "clean" {
		t.Fatalf("repository health = %#v", healthy)
	}

	contributor := filepath.Join(parent, "contributor")
	mustRunGit(t, parent, "clone", bareRemote, contributor)
	commitFile(t, contributor, "README.md", "initial\nremote\n", "remote update")
	mustRunGit(t, contributor, "push", "origin", "main")
	remoteHead := mustRunGit(t, bareRemote, "rev-parse", "refs/heads/main")

	fetched := decodeSingleJSON[repositoryResult](
		t,
		mustRunADB(
			t,
			parent,
			"repo",
			"fetch",
			repositoryID,
			"--workspace",
			root,
			"--org",
			"amazon",
			"--apply",
			"--format",
			"json",
		).stdout,
	)
	if fetched.Outcome != "applied" {
		t.Fatalf("repository fetch = %#v", fetched)
	}
	behind := decodeSingleJSON[repositoryResult](
		t,
		mustRunADB(
			t,
			parent,
			"repo",
			"health",
			repositoryID,
			"--workspace",
			root,
			"--org",
			"amazon",
			"--format",
			"json",
		).stdout,
	)
	if behind.Data.State != "behind" {
		t.Fatalf("repository health after fetch = %#v", behind)
	}

	updatePlan := decodeSingleJSON[repositoryResult](
		t,
		mustRunADB(
			t,
			parent,
			"repo",
			"update",
			repositoryID,
			"--workspace",
			root,
			"--org",
			"amazon",
			"--format",
			"json",
		).stdout,
	)
	if updatePlan.Outcome != "planned" {
		t.Fatalf("repository update plan = %#v", updatePlan)
	}
	updated := decodeSingleJSON[repositoryResult](
		t,
		mustRunADB(
			t,
			parent,
			"repo",
			"update",
			repositoryID,
			"--workspace",
			root,
			"--org",
			"amazon",
			"--apply",
			"--format",
			"json",
		).stdout,
	)
	if updated.Outcome != "applied" {
		t.Fatalf("repository update = %#v", updated)
	}
	if localHead := mustRunGit(t, clonePath, "rev-parse", "HEAD"); localHead != remoteHead {
		t.Fatalf("fast-forward HEAD = %q, want %q", localHead, remoteHead)
	}

	external := filepath.Join(parent, "external")
	mustRunGit(t, parent, "init", external)
	commitFile(t, external, "EXTERNAL.md", "authored\n", "external initial")
	mustRunGit(
		t,
		external,
		"remote",
		"add",
		"origin",
		"https://github.com/example/adopted.git",
	)
	adoptPlan := decodeSingleJSON[repositoryResult](
		t,
		mustRunADB(
			t,
			parent,
			"repo",
			"adopt",
			external,
			"--workspace",
			root,
			"--org",
			"amazon",
			"--format",
			"json",
		).stdout,
	)
	if adoptPlan.Outcome != "planned" {
		t.Fatalf("repository adopt plan = %#v", adoptPlan)
	}
	if _, err := os.Stat(filepath.Join(external, ".aidb")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("adopt plan mutated external clone: %v", err)
	}
	adopted := decodeSingleJSON[repositoryResult](
		t,
		mustRunADB(
			t,
			parent,
			"repo",
			"adopt",
			external,
			"--workspace",
			root,
			"--org",
			"amazon",
			"--apply",
			"--format",
			"json",
		).stdout,
	)
	if adopted.Outcome != "applied" ||
		!adopted.Data.Repository.ExternalClone ||
		adopted.Data.Repository.ClonePath != external {
		t.Fatalf("repository adoption = %#v", adopted)
	}
	if content, err := os.ReadFile(filepath.Join(external, "EXTERNAL.md")); err != nil ||
		string(content) != "authored\n" {
		t.Fatalf("adoption changed authored content: %q, %v", content, err)
	}

	finalList := decodeSingleJSON[repositoryResult](
		t,
		mustRunADB(
			t,
			parent,
			"repo",
			"list",
			"--workspace",
			root,
			"--org",
			"amazon",
			"--format",
			"json",
		).stdout,
	)
	if len(finalList.Data.Repositories) != 2 {
		t.Fatalf("repository list after adoption = %#v", finalList)
	}
}
