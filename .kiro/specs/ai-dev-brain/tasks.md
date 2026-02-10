# Implementation Plan: AI Dev Brain

## Overview

This implementation plan breaks down the AI Dev Brain system into discrete coding tasks. The system will be implemented in Go as a single compiled binary (`adb`) using Cobra for CLI, Viper for configuration, and gopkg.in/yaml.v3 for YAML serialization. Property-based tests use pgregory.net/rapid. Tasks are organized to build incrementally: core infrastructure first, then services, then integration layers.

## Tasks

- [ ] 1. Project Setup and Core Infrastructure
  - [ ] 1.1 Initialize Go module and project structure
    - Create go.mod with module path
    - Create directory structure: cmd/adb/, internal/cli/, internal/core/, internal/storage/, internal/integration/, pkg/models/
    - Add dependencies: github.com/spf13/cobra, github.com/spf13/viper, gopkg.in/yaml.v3, pgregory.net/rapid
    - Create cmd/adb/main.go entry point with root Cobra command
    - _Requirements: 15.1, 15.2_

  - [ ] 1.2 Implement shared models and types
    - Create pkg/models/task.go with TaskType, TaskStatus, Priority constants and Task struct
    - Create pkg/models/config.go with GlobalConfig, RepoConfig, MergedConfig structs
    - Create pkg/models/communication.go with Communication, CommunicationTag types
    - Create pkg/models/knowledge.go with Decision, HandoffDocument, ExtractedKnowledge structs
    - _Requirements: 8.1, 14.1_

  - [ ] 1.3 Implement Configuration Manager with Viper
    - Create internal/core/config.go with ConfigurationManager interface and viperConfigManager implementation
    - Implement LoadGlobalConfig() reading .taskconfig via Viper
    - Implement LoadRepoConfig() reading per-repo .taskrc files
    - Implement GetMergedConfig() with precedence: .taskrc > .taskconfig > defaults
    - Implement ValidateConfig() with clear error messages
    - _Requirements: 15.1, 15.3, 15.4, 15.5_

  - [ ] 1.4 Write property tests for Configuration Manager
    - **Property 9: Configuration Precedence Merging**
    - **Property 10: Configuration Validation**
    - **Validates: Requirements 15.3, 15.4**

  - [ ] 1.5 Implement Task ID Generator
    - Create internal/core/taskid.go with GenerateTaskID() function
    - Read/write counter from .task_counter file using os.ReadFile/os.WriteFile
    - Support configurable prefix (default: TASK)
    - Use file locking for atomic counter increment
    - _Requirements: 2.1_

  - [ ] 1.6 Write property test for Task ID uniqueness
    - **Property 1: Task ID Uniqueness**
    - **Validates: Requirements 2.1, 9.2**

- [ ] 2. Checkpoint - Core infrastructure tests pass
  - Ensure all tests pass with `go test ./...`, ask the user if questions arise.

- [ ] 3. Storage Layer Implementation
  - [ ] 3.1 Implement Backlog Manager
    - Create internal/storage/backlog.go with BacklogManager interface and fileBacklogManager implementation
    - Implement YAML serialization/deserialization using gopkg.in/yaml.v3 for backlog.yaml
    - Implement AddTask(), UpdateTask(), RemoveTask(), GetTask()
    - Implement FilterTasks() with status, priority, owner, repo, tags filters
    - Implement Load() and Save() for file persistence
    - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6_

  - [ ] 3.2 Write property tests for Backlog Manager
    - **Property 5: Backlog Serialization Round-Trip**
    - **Property 6: Backlog Filter Correctness**
    - **Validates: Requirements 8.3, 8.6**

  - [ ] 3.3 Implement Context Manager
    - Create internal/storage/context.go with ContextManager interface and fileContextManager implementation
    - Implement InitializeContext(), LoadContext(), UpdateContext(), PersistContext()
    - Implement GetContextForAI() to assemble AIContext from context.md, notes.md, and communications/
    - Use os.MkdirAll for directory creation, os.ReadFile/os.WriteFile for file I/O
    - _Requirements: 14.1, 14.2, 14.3, 14.4, 14.5_

  - [ ] 3.4 Write property test for Context persistence
    - **Property 14: Context Persistence Round-Trip**
    - **Validates: Requirements 14.1, 14.3**

  - [ ] 3.5 Implement Communication Manager
    - Create internal/storage/communication.go with CommunicationManager interface and fileCommunicationManager implementation
    - Implement AddCommunication() with filename format YYYY-MM-DD-source-contact-topic.md
    - Implement SearchCommunications() for content, date, source, contact queries using filepath.Walk
    - Implement tag extraction and tracking (requirement, decision, blocker, question, action_item)
    - _Requirements: 4.1, 4.3, 4.4, 4.5_

  - [ ] 3.6 Write property tests for Communication Manager
    - **Property 7: Communication Filename Format**
    - **Property 8: Communication Search Round-Trip**
    - **Validates: Requirements 4.1, 4.4**

- [ ] 4. Checkpoint - Storage layer tests pass
  - Ensure all tests pass with `go test ./...`, ask the user if questions arise.

- [ ] 5. Git Worktree Integration
  - [ ] 5.1 Implement Git Worktree Manager
    - Create internal/integration/worktree.go with GitWorktreeManager interface and gitWorktreeManager implementation
    - Implement CreateWorktree() using os/exec to run `git worktree add`
    - Implement RemoveWorktree(), ListWorktrees(), GetWorktreeForTask()
    - Support path structure: repos/{platform}/{org}/{repo}/work/TASK-XXXXX
    - _Requirements: 2.3, 9.1, 9.3, 9.4, 9.5_

  - [ ] 5.2 Write property tests for Git Worktree Manager
    - **Property 12: Multi-Repository Worktree Creation**
    - **Property 13: Repository Path Structure**
    - **Validates: Requirements 9.1, 9.4**

- [ ] 6. Bootstrap System
  - [ ] 6.1 Implement Bootstrap System
    - Create internal/core/bootstrap.go with BootstrapSystem interface and bootstrapSystem implementation
    - Implement Bootstrap() to create ticket folder structure using os.MkdirAll
    - Create communications/, notes.md, context.md, design.md, status.yaml
    - Integrate with Task ID Generator and Git Worktree Manager
    - _Requirements: 2.1, 2.2, 2.3, 2.6_

  - [ ] 6.2 Write property tests for Bootstrap System
    - **Property 2: Task Bootstrap Structure Completeness**
    - **Property 3: Template Application by Type**
    - **Validates: Requirements 2.2, 2.3, 2.6, 11.1**

  - [ ] 6.3 Implement Template Manager
    - Create internal/core/templates.go with TemplateManager interface and templateManager implementation
    - Implement ApplyTemplate() for feat, bug, spike, refactor types using text/template
    - Support per-repo .taskrc template overrides via Viper
    - Support custom template registration
    - _Requirements: 11.1, 11.2, 11.3, 11.4, 11.5_

- [ ] 7. Checkpoint - Bootstrap and worktree tests pass
  - Ensure all tests pass with `go test ./...`, ask the user if questions arise.

- [ ] 8. Task Manager Core
  - [ ] 8.1 Implement Task Manager
    - Create internal/core/taskmanager.go with TaskManager interface and taskManager implementation
    - Implement CreateTask() orchestrating bootstrap, worktree, backlog
    - Implement ResumeTask() loading context and worktree
    - Implement GetTasksByStatus(), GetAllTasks(), GetTask()
    - Implement ReorderPriorities()
    - _Requirements: 1.1, 1.2, 1.5, 1.6_

  - [ ] 8.2 Implement Archive/Unarchive functionality
    - Add ArchiveTask() to Task Manager
    - Generate handoff.md on archive using HandoffDocument struct
    - Update status to archived in backlog
    - Add UnarchiveTask() to restore previous status
    - _Requirements: 1.3, 1.4_

  - [ ] 8.3 Write property test for Archive/Unarchive
    - **Property 4: Archive/Unarchive Round-Trip**
    - **Validates: Requirements 1.3, 1.4**

- [ ] 9. Knowledge Extraction System
  - [ ] 9.1 Implement Knowledge Extractor
    - Create internal/core/knowledge.go with KnowledgeExtractor interface and knowledgeExtractor implementation
    - Implement ExtractFromTask() to read communications, notes, context, design.md
    - Implement GenerateHandoff() to create handoff.md
    - Implement UpdateWiki() to feed knowledge to docs/wiki/
    - _Requirements: 6.1, 6.2, 6.3, 6.4_

  - [ ] 9.2 Implement ADR Manager
    - Add CreateADR() to Knowledge Extractor
    - Generate ADR files with Status, Date, Source, Context, Decision, Consequences, Alternatives using text/template
    - Track provenance with "learned from TASK-XXXXX"
    - _Requirements: 6.5, 6.6_

  - [ ] 9.3 Write property tests for Knowledge Extraction
    - **Property 15: Knowledge Provenance Tracking**
    - **Property 16: ADR Creation Format**
    - **Validates: Requirements 6.5, 6.6**

- [ ] 10. Checkpoint - Task manager and knowledge extraction tests pass
  - Ensure all tests pass with `go test ./...`, ask the user if questions arise.

- [ ] 11. AI Context and Design Document Generators
  - [ ] 11.1 Implement AI Context Generator
    - Create internal/core/aicontext.go with AIContextGenerator interface and aiContextGenerator implementation
    - Implement GenerateContextFile() for CLAUDE.md and kiro.md using text/template
    - Assemble sections: overview, structure, conventions, glossary, decisions, tasks, contacts
    - Implement SyncContext() to regenerate from all sources
    - _Requirements: 16.1, 16.2, 16.3, 16.4, 16.5, 16.6_

  - [ ] 11.2 Write property tests for AI Context Generator
    - **Property 20: AI Context File Content Completeness**
    - **Property 21: AI Context File Sync Consistency**
    - **Validates: Requirements 16.1, 16.2, 16.3, 16.4, 16.5, 16.6**

  - [ ] 11.3 Implement Task Design Document Generator
    - Create internal/core/designdoc.go with TaskDesignDocGenerator interface and taskDesignDocGenerator implementation
    - Implement InitializeDesignDoc() to create design.md on bootstrap
    - Implement PopulateFromContext() to pull wiki, ADR, requirement context
    - Implement UpdateDesignDoc() for architecture and decision updates
    - Implement ExtractFromCommunications() for technical decision extraction
    - _Requirements: 17.1, 17.2, 17.3, 17.4, 17.5, 17.6_

  - [ ] 11.4 Write property tests for Task Design Document Generator
    - **Property 22: Task Design Document Bootstrap**
    - **Property 23: Task Design Document Context Population**
    - **Property 24: Technical Decision Extraction**
    - **Property 25: Design Document as Knowledge Source**
    - **Validates: Requirements 17.1, 17.2, 17.4, 17.6**

- [ ] 12. Checkpoint - AI context and design doc tests pass
  - Ensure all tests pass with `go test ./...`, ask the user if questions arise.

- [ ] 13. Update Generator and Conflict Detection
  - [ ] 13.1 Implement Update Generator
    - Create internal/core/updategen.go with UpdateGenerator interface and updateGenerator implementation
    - Implement GenerateUpdates() to review task context
    - Generate chronologically ordered PlannedMessage slices using sort.Slice
    - Identify information needed and who to contact
    - Format for email, Slack, Teams (no auto-send — return UpdatePlan only)
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6_

  - [ ] 13.2 Write property tests for Update Generator
    - **Property 18: Update Message Chronological Order**
    - **Property 19: No Auto-Send Invariant**
    - **Validates: Requirements 5.2, 5.6**

  - [ ] 13.3 Implement Conflict Detector
    - Create internal/core/conflict.go with ConflictDetector interface and conflictDetector implementation
    - Implement CheckForConflicts() against ADRs, previous decisions, requirements
    - Return []Conflict with Type, Source, Description, Recommendation, Severity
    - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5_

  - [ ] 13.4 Write property test for Conflict Detector
    - **Property 17: Conflict Reporting Format**
    - **Validates: Requirements 7.4**

- [ ] 14. Offline Mode Support
  - [ ] 14.1 Implement Offline Manager
    - Create internal/integration/offline.go with OfflineManager interface and offlineManager implementation
    - Implement IsOnline() using net.DialTimeout for connectivity detection
    - Implement QueueOperation() persisting to a JSON queue file
    - Implement SyncPendingOperations() with retry and exponential backoff
    - _Requirements: 10.1, 10.2, 10.3, 10.4, 10.5_

  - [ ] 14.2 Write property test for Offline Manager
    - **Property 11: Offline Operation Queue and Sync**
    - **Validates: Requirements 10.3, 10.5**

- [ ] 15. Checkpoint - Update generator, conflict detection, offline tests pass
  - Ensure all tests pass with `go test ./...`, ask the user if questions arise.

- [ ] 16. CLI Layer (Cobra Commands)
  - [ ] 16.1 Implement root command and feat/bug/spike/refactor subcommands
    - Create internal/cli/root.go with root Cobra command (`adb`)
    - Create internal/cli/feat.go implementing `adb feat "branch-name"` (also supports `adb bug`, `adb spike`, `adb refactor` via shared logic)
    - Wire Cobra commands to TaskManager.CreateTask()
    - _Requirements: 1.1, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6_

  - [ ] 16.2 Implement task lifecycle commands
    - Create internal/cli/resume.go implementing `adb resume TASK-XXXXX`
    - Create internal/cli/archive.go implementing `adb archive TASK-XXXXX`
    - Create internal/cli/unarchive.go implementing `adb unarchive TASK-XXXXX`
    - Wire to TaskManager.ResumeTask(), ArchiveTask(), UnarchiveTask()
    - _Requirements: 1.2, 1.3, 1.4_

  - [ ] 16.3 Implement status and priority commands
    - Create internal/cli/status.go implementing `adb status`
    - Create internal/cli/priority.go implementing `adb priority`
    - Wire to TaskManager.GetTasksByStatus(), ReorderPriorities()
    - _Requirements: 1.5, 1.6_

  - [ ] 16.4 Implement utility commands
    - Create internal/cli/update.go implementing `adb update`
    - Create internal/cli/synccontext.go implementing `adb sync-context`
    - Wire to UpdateGenerator.GenerateUpdates(), AIContextGenerator.SyncContext()
    - _Requirements: 5.1, 16.6_

- [ ] 17. Tab Manager Integration
  - [ ] 17.1 Implement Tab Manager
    - Create internal/integration/tab.go with TabManager interface and tabManager implementation
    - Implement RenameTab() using ANSI escape sequences for terminal, VS Code/Kiro detection via env vars
    - Implement RestoreTab() for session end
    - Implement DetectEnvironment() checking TERM_PROGRAM, VSCODE_PID env vars
    - Handle failures gracefully: return error but don't block operations
    - _Requirements: 13.1, 13.2, 13.3, 13.4_

- [ ] 18. Screenshot OCR Pipeline
  - [ ] 18.1 Implement Screenshot Pipeline
    - Create internal/integration/screenshot.go with ScreenshotPipeline interface and screenshotPipeline implementation
    - Implement Capture() using os/exec to call OS-specific screen capture (screencapture on macOS, import on Linux)
    - Implement ProcessScreenshot() with AI OCR integration placeholder
    - Implement FileContent() to auto-file to appropriate location
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6_

- [ ] 19. CLI Executor and Taskfile Runner
  - [ ] 19.1 Implement CLI Executor
    - Create internal/integration/cliexec.go with CLIExecutor interface and cliExecutor implementation
    - Implement ResolveAlias() to look up CLI aliases from .taskconfig and return expanded command + default_args
    - Implement BuildEnv() to inject ADB_TASK_ID, ADB_BRANCH, ADB_WORKTREE_PATH, ADB_TICKET_PATH into subprocess env when task context is active
    - Implement Exec() using os/exec.Command to invoke external CLIs, streaming stdout/stderr, returning exit code
    - Implement ListAliases() to return all configured aliases for display
    - Implement LogFailure() to append failure details to the task's context.md when a task is active
    - Support piping by delegating to `sh -c` (or `cmd /c` on Windows) when pipe characters are detected
    - _Requirements: 18.1, 18.3, 18.5, 18.6, 18.7, 18.8_

  - [ ]* 19.2 Write property tests for CLI Executor
    - **Property 26: CLI Argument Passthrough**
    - **Property 27: Task Context Environment Injection**
    - **Property 29: CLI Alias Resolution**
    - **Property 30: CLI Failure Propagation**
    - **Validates: Requirements 18.1, 18.3, 18.5, 18.6**

  - [ ] 19.3 Implement Taskfile Runner
    - Create internal/integration/taskfilerunner.go with TaskfileRunner interface and taskfileRunner implementation
    - Implement Discover() to parse Taskfile.yaml using gopkg.in/yaml.v3
    - Implement ListTasks() to return task names and descriptions from parsed Taskfile
    - Implement Run() to execute a named task, passing args and injecting task context env vars via CLIExecutor
    - Search for Taskfile.yaml in dev-brain root and current repository directory
    - _Requirements: 18.2, 18.4_

  - [ ]* 19.4 Write property test for Taskfile Runner
    - **Property 28: Taskfile Task Discovery**
    - **Validates: Requirements 18.2, 18.4**

  - [ ] 19.5 Implement `adb exec` and `adb run` Cobra commands
    - Create internal/cli/exec.go implementing `adb exec <cli> [args...]`
    - Wire to CLIExecutor.Exec() with alias resolution and task context injection
    - Display configured aliases when `adb exec` is called with no arguments
    - Create internal/cli/run.go implementing `adb run <task> [args...]`
    - Wire to TaskfileRunner.Run() with task context injection
    - Implement `adb run --list` flag to display available Taskfile tasks
    - _Requirements: 18.1, 18.2, 18.4, 18.8_

  - [ ] 19.6 Update Configuration Manager for CLI aliases
    - Add CLIAliases field to GlobalConfig struct in pkg/models/config.go
    - Update LoadGlobalConfig() to read cli_aliases section from .taskconfig via Viper
    - Pass loaded aliases to CLIExecutor during dependency wiring
    - _Requirements: 18.5_

- [ ] 20. Checkpoint - CLI Executor and Taskfile Runner tests pass
  - Ensure all tests pass with `go test ./...`, ask the user if questions arise.

- [ ] 21. Documentation Structure Setup
  - [ ] 21.1 Create documentation templates
    - Create embedded templates using Go embed for: docs/stakeholders.md, docs/contacts.md, docs/glossary.md
    - Create ADR template in docs/decisions/
    - Create docs/runbooks/ and docs/wiki/ directory structure scaffolding
    - Implement init command or bootstrap logic to create these on first run
    - _Requirements: 12.1, 12.2, 12.3, 12.4, 12.5, 12.6_

- [ ] 22. Final Integration and Wiring
  - [ ] 22.1 Wire all components together
    - Update cmd/adb/main.go to initialize all dependencies and wire into Cobra commands
    - Create internal/app.go with App struct holding all service dependencies
    - Implement dependency injection: App creates concrete implementations and passes interfaces to CLI commands
    - Wire CLIExecutor and TaskfileRunner into `adb exec` and `adb run` commands with config-loaded aliases
    - Build binary with `go build -o adb ./cmd/adb/`
    - _Requirements: All_

  - [ ] 22.2 Write integration tests
    - End-to-end task lifecycle: Create → Work → Archive → Unarchive → Resume
    - Multi-repo workflow: Create task spanning 3 repos
    - Offline/online transition: Queue operations, verify sync
    - Knowledge feedback: Archive task, verify wiki and ADR updates
    - CLI execution within task: Create task, run `adb exec` and `adb run`, verify env vars and failure logging
    - Use testing.T with t.TempDir() for isolated test environments
    - _Requirements: All_

- [ ] 23. Final Checkpoint - All tests pass
  - Ensure all tests pass with `go test ./...`, ask the user if questions arise.

## Notes

- All tasks are required for comprehensive implementation
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties using pgregory.net/rapid
- Unit tests validate specific examples and edge cases using Go's built-in testing package
- The system compiles to a single binary (`adb`) via `go build -o adb ./cmd/adb/`
- Users place the binary in `~/.local/bin/` (Linux/Mac) or add to PATH on Windows
