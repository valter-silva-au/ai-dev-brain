package e2e

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func decodeSingleJSON[T any](t *testing.T, output string) T {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(output))
	var result T
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode JSON result: %v\n%s", err, output)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("output contains more than one JSON value: %v\n%s", err, output)
	}
	return result
}

func initializeV3Workspace(t *testing.T, parent string, root string) {
	t.Helper()

	result := decodeSingleJSON[foundationResult](
		t,
		mustRunADB(
			t,
			parent,
			"init",
			"boundary",
			root,
			"--name",
			filepath.Base(root),
			"--apply",
			"--format",
			"json",
		).stdout,
	)
	if result.Outcome != "applied" {
		t.Fatalf("workspace initialization outcome = %q, want applied", result.Outcome)
	}
}

func initializeV3Organization(
	t *testing.T,
	parent string,
	root string,
	slug string,
	name string,
) {
	t.Helper()

	result := decodeSingleJSON[organizationResult](
		t,
		mustRunADB(
			t,
			parent,
			"org",
			"init",
			slug,
			"--workspace",
			root,
			"--name",
			name,
			"--apply",
			"--format",
			"json",
		).stdout,
	)
	if result.Outcome != "applied" {
		t.Fatalf("organization initialization outcome = %q, want applied", result.Outcome)
	}
}

func mustRunGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	command := exec.Command("git", args...)
	command.Dir = dir
	command.Env = append(
		os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf(
			"git %s failed: %v\n%s",
			strings.Join(args, " "),
			err,
			output,
		)
	}
	return strings.TrimSpace(string(output))
}

func commitFile(
	t *testing.T,
	repositoryPath string,
	relativePath string,
	content string,
	message string,
) {
	t.Helper()

	target := filepath.Join(repositoryPath, relativePath)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create commit file parent: %v", err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatalf("write commit file: %v", err)
	}
	mustRunGit(t, repositoryPath, "add", "--", relativePath)
	mustRunGit(
		t,
		repositoryPath,
		"-c",
		"user.name=ADB E2E",
		"-c",
		"user.email=adb-e2e@example.invalid",
		"commit",
		"-m",
		message,
	)
}
