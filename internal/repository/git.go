package repository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Invocation struct {
	Dir  string
	Args []string
}

type RunResult struct {
	Stdout string
	Stderr string
}

type Runner interface {
	Run(context.Context, Invocation) (RunResult, error)
}

type Git interface {
	Inventory(context.Context, string, string) (Inventory, error)
	Clone(context.Context, string, string, string) error
	Init(context.Context, string) error
	AddRemote(context.Context, string, string, string) error
	SetRemoteURL(context.Context, string, string, string) error
	Fetch(context.Context, string, string) error
	FastForward(context.Context, string, string, string) error
	Worktrees(context.Context, string) ([]GitWorktree, error)
	AddWorktree(context.Context, string, string, string) error
	RemoveWorktree(context.Context, string, string) error
}

type ExecRunner struct {
	Executable string
	WaitDelay  time.Duration
}

type Client struct {
	runner Runner
}

type RemoteState struct {
	Name     string
	FetchURL string
	PushURL  string
}

type Inventory struct {
	Path          string
	Root          string
	GitDir        string
	CommonDir     string
	IsRepository  bool
	IsWorktree    bool
	Detached      bool
	Branch        string
	Upstream      string
	DefaultBranch string
	Dirty         bool
	Ahead         int
	Behind        int
	Diverged      bool
	Remotes       []RemoteState
}

type GitWorktree struct {
	Path     string
	HEAD     string
	Branch   string
	Bare     bool
	Detached bool
	Locked   bool
	Prunable bool
}

type CommandError struct {
	Invocation Invocation
	Stderr     string
	Cause      error
}

var ErrAuthentication = errors.New("git authentication failed")

func (failure *CommandError) Error() string {
	message := "git " + strings.Join(failure.Invocation.Args, " ") + " failed"
	if failure.Stderr != "" {
		message += ": " + failure.Stderr
	}
	return message
}

func (failure *CommandError) Unwrap() error {
	return failure.Cause
}

func NewClient(runner Runner) *Client {
	if runner == nil {
		runner = &ExecRunner{}
	}
	return &Client{runner: runner}
}

func (runner *ExecRunner) Run(
	ctx context.Context,
	invocation Invocation,
) (RunResult, error) {
	executable := runner.Executable
	if executable == "" {
		executable = "git"
	}
	command := exec.CommandContext(ctx, executable, invocation.Args...)
	command.Dir = invocation.Dir
	command.Env = promptDisabledEnvironment(os.Environ())
	command.WaitDelay = runner.WaitDelay
	if command.WaitDelay == 0 {
		command.WaitDelay = 5 * time.Second
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := RunResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if err != nil {
		cause := error(err)
		if authenticationFailure(result.Stderr) {
			cause = errors.Join(err, ErrAuthentication)
		}
		return result, &CommandError{
			Invocation: invocation,
			Stderr:     strings.TrimSpace(result.Stderr),
			Cause:      cause,
		}
	}
	return result, nil
}

func (client *Client) Inventory(
	ctx context.Context,
	path string,
	canonicalRemote string,
) (Inventory, error) {
	if path == "" {
		return Inventory{}, errors.New("repository inventory path is required")
	}
	if canonicalRemote == "" {
		return Inventory{}, errors.New(
			"repository canonical remote name is required",
		)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Inventory{}, fmt.Errorf(
			"resolve repository inventory path: %w",
			err,
		)
	}
	inventory := Inventory{Path: filepath.Clean(absolute)}
	inside, err := client.output(
		ctx,
		inventory.Path,
		"rev-parse",
		"--is-inside-work-tree",
	)
	if err != nil {
		// `git rev-parse --is-inside-work-tree` exits non-zero when the path is
		// not in a work tree, so a *command* failure is this probe's answer:
		// leave IsRepository false and report no error, because every caller
		// branches on IsRepository.
		//
		// Failing to run git at all is not an answer about the path. Reported as
		// "not a repository" it would make every path on a machine without git
		// look un-adoptable, and `adb repo adopt` / `repo health` would blame the
		// path instead of the missing binary. Same for a cancelled context.
		if probeUnanswered(ctx, err) {
			return Inventory{}, fmt.Errorf(
				"probe repository work tree %q: %w",
				inventory.Path,
				err,
			)
		}
		return inventory, nil
	}
	if strings.TrimSpace(inside) != "true" {
		return inventory, nil
	}
	inventory.IsRepository = true

	root, err := client.requiredOutput(
		ctx,
		inventory.Path,
		"rev-parse",
		"--show-toplevel",
	)
	if err != nil {
		return Inventory{}, err
	}
	inventory.Root = filepath.Clean(root)
	gitDir, err := client.requiredOutput(
		ctx,
		inventory.Path,
		"rev-parse",
		"--git-dir",
	)
	if err != nil {
		return Inventory{}, err
	}
	commonDir, err := client.requiredOutput(
		ctx,
		inventory.Path,
		"rev-parse",
		"--git-common-dir",
	)
	if err != nil {
		return Inventory{}, err
	}
	inventory.GitDir = resolveGitPath(inventory.Root, gitDir)
	inventory.CommonDir = resolveGitPath(inventory.Root, commonDir)
	inventory.IsWorktree = inventory.GitDir != inventory.CommonDir

	remoteOutput, err := client.requiredOutput(
		ctx,
		inventory.Path,
		"remote",
	)
	if err != nil {
		return Inventory{}, err
	}
	remoteNames := nonemptyLines(remoteOutput)
	sort.Strings(remoteNames)
	inventory.Remotes = make([]RemoteState, 0, len(remoteNames))
	for _, name := range remoteNames {
		fetchURL, err := client.requiredOutput(
			ctx,
			inventory.Path,
			"remote",
			"get-url",
			name,
		)
		if err != nil {
			return Inventory{}, err
		}
		pushURL, err := client.requiredOutput(
			ctx,
			inventory.Path,
			"remote",
			"get-url",
			"--push",
			name,
		)
		if err != nil {
			return Inventory{}, err
		}
		inventory.Remotes = append(inventory.Remotes, RemoteState{
			Name:     name,
			FetchURL: fetchURL,
			PushURL:  pushURL,
		})
	}

	defaultBranch, err := client.output(
		ctx,
		inventory.Path,
		"symbolic-ref",
		"--quiet",
		"--short",
		"refs/remotes/"+canonicalRemote+"/HEAD",
	)
	if err == nil {
		inventory.DefaultBranch = strings.TrimPrefix(
			strings.TrimSpace(defaultBranch),
			canonicalRemote+"/",
		)
	}

	status, err := client.requiredOutput(
		ctx,
		inventory.Path,
		"status",
		"--porcelain=v2",
		"--branch",
		"--untracked-files=normal",
	)
	if err != nil {
		return Inventory{}, err
	}
	parsePorcelainStatus(status, &inventory)
	return inventory, nil
}

func (client *Client) Clone(
	ctx context.Context,
	dir string,
	remote string,
	destination string,
) error {
	if dir == "" || remote == "" || destination == "" {
		return errors.New(
			"git clone requires directory, remote, and destination",
		)
	}
	_, err := client.runner.Run(ctx, Invocation{
		Dir:  dir,
		Args: []string{"clone", "--", remote, destination},
	})
	return err
}

func (client *Client) Init(ctx context.Context, path string) error {
	if path == "" {
		return errors.New("git init path is required")
	}
	_, err := client.runner.Run(ctx, Invocation{
		Dir:  filepath.Dir(path),
		Args: []string{"init", "--", path},
	})
	return err
}

func (client *Client) Fetch(
	ctx context.Context,
	path string,
	remote string,
) error {
	if path == "" || remote == "" {
		return errors.New("git fetch requires path and remote")
	}
	_, err := client.runner.Run(ctx, Invocation{
		Dir:  path,
		Args: []string{"fetch", "--prune", "--no-tags", remote},
	})
	return err
}

func (client *Client) AddRemote(
	ctx context.Context,
	path string,
	name string,
	remote string,
) error {
	if path == "" || name == "" || remote == "" {
		return errors.New("git remote add requires path, name, and remote")
	}
	_, err := client.runner.Run(ctx, Invocation{
		Dir:  path,
		Args: []string{"remote", "add", name, remote},
	})
	return err
}

func (client *Client) SetRemoteURL(
	ctx context.Context,
	path string,
	name string,
	remote string,
) error {
	if path == "" || name == "" || remote == "" {
		return errors.New("git remote set-url requires path, name, and remote")
	}
	_, err := client.runner.Run(ctx, Invocation{
		Dir:  path,
		Args: []string{"remote", "set-url", name, remote},
	})
	return err
}

func (client *Client) FastForward(
	ctx context.Context,
	path string,
	remote string,
	branch string,
) error {
	if path == "" || remote == "" || branch == "" {
		return errors.New(
			"git fast-forward requires path, remote, and branch",
		)
	}
	_, err := client.runner.Run(ctx, Invocation{
		Dir: path,
		Args: []string{
			"merge",
			"--ff-only",
			remote + "/" + branch,
		},
	})
	return err
}

func (client *Client) Worktrees(
	ctx context.Context,
	path string,
) ([]GitWorktree, error) {
	if path == "" {
		return nil, errors.New("git worktree list path is required")
	}
	output, err := client.requiredOutput(
		ctx,
		path,
		"worktree",
		"list",
		"--porcelain",
	)
	if err != nil {
		return nil, err
	}
	return parseWorktrees(output), nil
}

func (client *Client) AddWorktree(
	ctx context.Context,
	repositoryPath string,
	worktreePath string,
	branch string,
) error {
	if repositoryPath == "" || worktreePath == "" || branch == "" {
		return errors.New(
			"git worktree add requires repository, path, and branch",
		)
	}
	_, err := client.runner.Run(ctx, Invocation{
		Dir: repositoryPath,
		Args: []string{
			"worktree",
			"add",
			"--",
			worktreePath,
			branch,
		},
	})
	return err
}

func (client *Client) RemoveWorktree(
	ctx context.Context,
	repositoryPath string,
	worktreePath string,
) error {
	if repositoryPath == "" || worktreePath == "" {
		return errors.New(
			"git worktree remove requires repository and path",
		)
	}
	_, err := client.runner.Run(ctx, Invocation{
		Dir:  repositoryPath,
		Args: []string{"worktree", "remove", "--", worktreePath},
	})
	return err
}

// probeUnanswered reports whether err means git never delivered a verdict, as
// opposed to delivering a negative one through a non-zero exit status. Only
// these cases may be propagated by a probe whose failure is otherwise its
// answer:
//
//   - git could not be executed at all (absent from PATH, a configured path that
//     does not exist, or one this user cannot execute), and
//   - the context was cancelled or timed out.
//
// A real "not a git repository" exit is an *exec.ExitError, which matches none
// of these.
func probeUnanswered(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return true
	}
	return errors.Is(err, exec.ErrNotFound) ||
		errors.Is(err, os.ErrNotExist) ||
		errors.Is(err, os.ErrPermission) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

func (client *Client) output(
	ctx context.Context,
	dir string,
	args ...string,
) (string, error) {
	result, err := client.runner.Run(ctx, Invocation{
		Dir:  dir,
		Args: append([]string(nil), args...),
	})
	return strings.TrimSpace(result.Stdout), err
}

func (client *Client) requiredOutput(
	ctx context.Context,
	dir string,
	args ...string,
) (string, error) {
	output, err := client.output(ctx, dir, args...)
	if err != nil {
		return "", err
	}
	return output, nil
}

func parsePorcelainStatus(output string, inventory *Inventory) {
	for _, line := range strings.Split(output, "\n") {
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			inventory.Branch = strings.TrimPrefix(line, "# branch.head ")
			if inventory.Branch == "(detached)" {
				inventory.Branch = ""
				inventory.Detached = true
			}
		case strings.HasPrefix(line, "# branch.upstream "):
			inventory.Upstream = strings.TrimPrefix(
				line,
				"# branch.upstream ",
			)
		case strings.HasPrefix(line, "# branch.ab "):
			fields := strings.Fields(strings.TrimPrefix(line, "# branch.ab "))
			if len(fields) == 2 {
				inventory.Ahead = parseSignedCount(fields[0])
				inventory.Behind = parseSignedCount(fields[1])
			}
		case line == "":
		case strings.HasPrefix(line, "#"):
		default:
			inventory.Dirty = true
		}
	}
	inventory.Diverged = inventory.Ahead > 0 && inventory.Behind > 0
}

func parseSignedCount(value string) int {
	value = strings.TrimPrefix(strings.TrimPrefix(value, "+"), "-")
	count, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return count
}

func resolveGitPath(root string, path string) string {
	path = filepath.FromSlash(strings.TrimSpace(path))
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(root, path))
}

func nonemptyLines(output string) []string {
	lines := make([]string, 0)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func parseWorktrees(output string) []GitWorktree {
	worktrees := make([]GitWorktree, 0)
	var current *GitWorktree
	flush := func() {
		if current != nil {
			worktrees = append(worktrees, *current)
			current = nil
		}
	}
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			flush()
			continue
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			current = &GitWorktree{
				Path: filepath.Clean(filepath.FromSlash(
					strings.TrimPrefix(line, "worktree "),
				)),
			}
		case current == nil:
		case strings.HasPrefix(line, "HEAD "):
			current.HEAD = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			current.Branch = strings.TrimPrefix(
				strings.TrimPrefix(line, "branch "),
				"refs/heads/",
			)
		case line == "bare":
			current.Bare = true
		case line == "detached":
			current.Detached = true
		case line == "locked" || strings.HasPrefix(line, "locked "):
			current.Locked = true
		case line == "prunable" || strings.HasPrefix(line, "prunable "):
			current.Prunable = true
		}
	}
	flush()
	return worktrees
}

func promptDisabledEnvironment(environment []string) []string {
	result := append([]string(nil), environment...)
	result = replaceEnvironmentValue(result, "GIT_TERMINAL_PROMPT", "0")
	if _, ok := environmentValue(result, "GIT_SSH_COMMAND"); !ok {
		result = append(result, "GIT_SSH_COMMAND=ssh -oBatchMode=yes")
	}
	return result
}

func replaceEnvironmentValue(
	environment []string,
	name string,
	value string,
) []string {
	prefix := name + "="
	for index, item := range environment {
		if strings.HasPrefix(item, prefix) {
			environment[index] = prefix + value
			return environment
		}
	}
	return append(environment, prefix+value)
}

func environmentValue(environment []string, name string) (string, bool) {
	prefix := name + "="
	for _, item := range environment {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix), true
		}
	}
	return "", false
}

func authenticationFailure(stderr string) bool {
	lower := strings.ToLower(stderr)
	for _, marker := range []string{
		"authentication failed",
		"permission denied (publickey)",
		"could not read username",
		"terminal prompts disabled",
		"repository not found",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

var _ Runner = (*ExecRunner)(nil)
var _ Git = (*Client)(nil)
