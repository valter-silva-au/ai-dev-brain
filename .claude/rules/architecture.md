---
paths:
  - "internal/**/*.go"
  - "pkg/**/*.go"
---

Architecture rules for this project:

- internal/core/ contains business logic and defines local interfaces for dependencies
- internal/storage/ implements persistence (BacklogManager, ContextManager, CommunicationManager)
- internal/integration/ implements external system interaction (git worktrees, CLI exec, offline detection)
- internal/cli/ implements Cobra commands, depends only on core interfaces via package vars
- pkg/models/ contains shared data types used across all packages
- internal/app.go wires everything together with the adapter pattern
- NEVER import storage or integration from core - use local interfaces instead
- New features should define interfaces in the consuming package
- All constructors return interfaces, not concrete types
- File-based storage produces human-readable, git-diffable YAML output
- Template rendering uses text/template, not string concatenation
- Adapter structs in app.go translate between package-local types
