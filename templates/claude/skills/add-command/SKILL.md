---
name: add-command
description: Scaffold a new Cobra CLI command following project patterns
argument-hint: "<command-name>"
allowed-tools: Read, Write, Edit, Bash, Glob
---

# Add Command Skill

Scaffold a new Cobra CLI command following the project's established patterns.

## Prerequisites

- The command name argument is **required**. If not provided, ask the user.

## Steps

### 1. Study existing patterns
Read these files to understand the project's CLI command patterns:
- `internal/cli/feat.go` - Example of a command using a package-level DI variable (`TaskMgr`)
- `internal/cli/resume.go` - Example of a simpler command with `cobra.ExactArgs(1)`
- `internal/cli/feat_test.go` - Example of test structure with mock dependencies

### 2. Create the command file
Create `internal/cli/<command-name>.go` with:

```go
package cli

import (
    "fmt"
    "github.com/spf13/cobra"
)

var <commandName>Cmd = &cobra.Command{
    Use:   "<command-name> <args>",
    Short: "<Short description>",
    Long:  `<Longer description of what the command does.>`,
    Args:  cobra.ExactArgs(1), // Adjust as needed
    RunE: func(cmd *cobra.Command, args []string) error {
        // Implementation
        return nil
    },
}

func init() {
    rootCmd.AddCommand(<commandName>Cmd)
}
```

- Use `RunE` (not `Run`) to support error propagation
- Add a package-level variable for any service dependency (e.g., `var SomeSvc core.SomeInterface`)
- Check for nil dependencies at the start of RunE
- Use `cobra.ExactArgs(N)` or `cobra.MinimumNArgs(N)` for argument validation

### 3. Create the test file
Create `internal/cli/<command-name>_test.go` with:

```go
package cli

import (
    "bytes"
    "testing"
)

func Test<CommandName>Command_RegistrationInRoot(t *testing.T) {
    subcommands := rootCmd.Commands()
    names := make(map[string]bool)
    for _, cmd := range subcommands {
        names[cmd.Name()] = true
    }
    if !names["<command-name>"] {
        t.Errorf("expected command %q to be registered", "<command-name>")
    }
}
```

- Follow the mock pattern from `feat_test.go` if the command uses DI
- Test argument validation, nil dependency handling, and happy path

### 4. Check DI wiring
If the command needs a service dependency:
- Add a package-level variable in the command file (e.g., `var SomeSvc core.SomeInterface`)
- Remind the user to wire it in `internal/app.go` inside `NewApp()`, in the "Wire CLI package-level variables" section (around line 104)

### 5. Verify
Run `go build ./cmd/adb/` to confirm the command compiles.
Run `go test ./internal/cli/ -run Test<CommandName> -v` to verify tests pass.
