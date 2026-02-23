---
name: add-interface
description: Scaffold a new interface following the project's architecture patterns
argument-hint: "<package> <interface-name>"
allowed-tools: Read, Write, Edit, Glob, Grep
---

# Add Interface Skill

Scaffold a new interface following the project's architecture patterns.

## Prerequisites

- Two arguments are required: package name and interface name.
  - First argument: package (one of: core, storage, integration)
  - Second argument: interface name (e.g., TaskManager, GitWorktreeManager)
- If arguments are missing, ask the user.

## Steps

### 1. Study existing patterns
Read existing interfaces in the target package directory to understand conventions:
- Use Glob to find `internal/<package>/*.go` files
- Read at least 2 interface definitions in that package
- Note naming conventions, method signatures, and constructor patterns

### 2. Create the interface definition file
Create `internal/<package>/<interface_name_snake>.go` with:

```go
package <package>

// <InterfaceName> defines the contract for <description>.
type <InterfaceName> interface {
    // Method signatures based on user requirements
}
```

### 3. Create the concrete implementation
In the same file or a separate file, create the struct and constructor:

```go
// <implName> implements <InterfaceName>.
type <implName> struct {
    // fields
}

// New<InterfaceName> creates a new <InterfaceName> implementation.
func New<InterfaceName>(/* params */) <InterfaceName> {
    return &<implName>{
        // field initialization
    }
}
```

- The constructor should return the interface type, not the concrete type
- Use unexported struct name (lowercase first letter) for the implementation

### 4. Create the test file
Create `internal/<package>/<interface_name_snake>_test.go` with:

```go
package <package>

import "testing"

func Test<ImplName>_Implements<InterfaceName>(t *testing.T) {
    var _ <InterfaceName> = &<implName>{}
}
```

Add basic tests for each method.

### 5. Handle cross-package dependencies
If the interface is in `core/` and needs types from `storage/` or `integration/`:
- Do NOT import those packages from core
- Instead, define a local interface in core that describes what is needed
- The adapter pattern is used in `internal/app.go` to bridge packages (see `worktreeAdapter`, `backlogStoreAdapter`)

### 6. Remind about wiring
Tell the user:
- Add the new service to the `App` struct in `internal/app.go`
- Wire it in `NewApp()` with proper initialization
- If CLI commands need it, add a package-level variable in `internal/cli/` and wire it in the "Wire CLI package-level variables" section of `NewApp()`
- If cross-package, create an adapter in `internal/app.go` following the existing adapter patterns
