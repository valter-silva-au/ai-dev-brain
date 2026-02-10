# Requirements Document

## Introduction

AI Dev Brain is an AI-powered developer productivity system that maximizes the effectiveness of AI coding assistants (Claude Code, Kiro) through rich context management. It provides a monorepo structure that wraps AI coding assistants with persistent context, task management, knowledge accumulation, and stakeholder communication tools. The system enables solo developers managing multiple projects/orgs/teams to maintain continuity across AI sessions while accumulating organizational knowledge.

## Glossary

- **Task**: A unit of work identified by a unique TASK-XXXXX ID, containing all context, communications, and artifacts related to that work item
- **Worktree**: A Git worktree providing isolated working directory for a specific task branch
- **Handoff_Document**: An auto-generated summary (handoff.md) created when archiving a task, capturing learnings and decisions for future reference
- **ADR**: Architecture Decision Record documenting significant technical decisions with context and rationale
- **Backlog**: A YAML-based central registry (backlog.yaml) tracking all tasks with their status, priority, and metadata
- **Context_File**: A markdown file (context.md) containing task-specific context for AI session continuity
- **Communications_Folder**: A directory containing chronological markdown files of stakeholder communications
- **Screenshot_Pipeline**: An automated system that captures screenshots, extracts text via OCR, and files content appropriately
- **Task_Bootstrap**: The process of initializing a new task with folder structure, git worktree, and AI session
- **Knowledge_Feedback**: The process of extracting learnings from completed tasks and updating documentation
- **AI_Context_File**: A root-level markdown file (CLAUDE.md or kiro.md) containing curated context for AI coding assistants including project overview, conventions, and key documentation pointers
- **Task_Design_Document**: A technical document (design.md) within each task folder containing architecture, diagrams, and technical decisions specific to that task
- **External_CLI**: A third-party command-line tool (e.g., aws, gh, git, docker) that can be invoked by the adb binary as a subprocess
- **CLI_Plugin**: A configured external CLI command or Taskfile task that is registered with adb and can be executed through the `adb exec` or `adb run` subcommands
- **Taskfile_Integration**: The ability for adb to discover and execute tasks defined in a Taskfile.yaml, acting as a wrapper that adds task context to Taskfile commands

## Requirements

### Requirement 1: Task Lifecycle Management

**User Story:** As a developer, I want to manage the complete lifecycle of tasks through CLI commands, so that I can efficiently start, resume, archive, and track work items.

#### Acceptance Criteria

1. WHEN a user executes `tkt feat "branch-name"` THEN THE Task_Manager SHALL create a new task with unique TASK-XXXXX ID, bootstrap the ticket folder structure, create a git worktree, and start an AI session
2. WHEN a user executes `tktr TASK-XXXXX` THEN THE Task_Manager SHALL resume the specified task, set terminal name to "TASK-XXXXX (branch-name)", and start an AI session in the existing worktree
3. WHEN a user executes `tkt archive TASK-XXXXX` THEN THE Task_Manager SHALL generate a handoff.md document, feed knowledge back to docs/wiki/, and update task status to archived
4. WHEN a user executes `tkt unarchive TASK-XXXXX` THEN THE Task_Manager SHALL restore the archived task to its previous status and make it available for resumption
5. WHEN a user executes `tkt status` THEN THE Task_Manager SHALL display all tasks grouped by status (backlog, in_progress, blocked, review, done, archived)
6. WHEN a user executes `tkt priority` THEN THE Task_Manager SHALL allow reordering of task priorities in the backlog

### Requirement 2: Task Bootstrap Process

**User Story:** As a developer, I want new tasks to be automatically bootstrapped with proper structure, so that I can immediately begin productive work with full context.

#### Acceptance Criteria

1. WHEN bootstrapping a new task THEN THE Bootstrap_System SHALL generate a unique TASK-XXXXX ID using a central counter or external system integration
2. WHEN bootstrapping a new task THEN THE Bootstrap_System SHALL create the directory structure: tickets/TASK-XXXXX/ containing communications/, notes.md, context.md, and status.yaml
3. WHEN bootstrapping a new task THEN THE Bootstrap_System SHALL create a git worktree in the appropriate repos/platform/org/repo/work/TASK-XXXXX location
4. WHEN bootstrapping a new task THEN THE Bootstrap_System SHALL rename the VS Code/Kiro/terminal tab to "TASK-XXXXX (branch-name)"
5. WHEN bootstrapping a new task THEN THE Bootstrap_System SHALL start the configured AI coding assistant (Claude Code or Kiro) inside the worktree
6. WHEN a task type is specified (feat, bug, spike, refactor) THEN THE Bootstrap_System SHALL apply the corresponding task template

### Requirement 3: Screenshot OCR Pipeline

**User Story:** As a developer, I want to capture screenshots and have them automatically processed and filed, so that I can quickly capture information from any source without manual transcription.

#### Acceptance Criteria

1. WHEN a user triggers the screenshot hotkey THEN THE Screenshot_Pipeline SHALL capture the screen content
2. WHEN a screenshot is captured THEN THE OCR_Processor SHALL extract text from the image using AI-powered OCR
3. WHEN text is extracted THEN THE Content_Categorizer SHALL categorize and summarize the content based on context
4. WHEN content is categorized THEN THE File_Manager SHALL auto-file to the appropriate location (communications/, wiki/, etc.)
5. WHEN filing content THEN THE Content_Filter SHALL filter noise and extract only task-relevant information
6. IF the screenshot capture fails THEN THE Screenshot_Pipeline SHALL notify the user with an error message and suggested remediation

### Requirement 4: Communication Management

**User Story:** As a developer, I want to track all task-related communications in an organized manner, so that I can maintain context and reference past discussions.

#### Acceptance Criteria

1. WHEN a communication is added THEN THE Communication_Manager SHALL store it as a chronological markdown file in communications/ with naming format: YYYY-MM-DD-source-contact-topic.md
2. WHEN content is ingested (manual paste or screenshot) THEN THE AI_Filter SHALL extract relevant information and filter noise
3. WHEN processing communications THEN THE Communication_Manager SHALL track and tag: requirements, decisions, blockers, questions, and action items
4. WHEN a user queries communications THEN THE Communication_Manager SHALL provide searchable access to all stored communications for the task
5. THE Communication_Manager SHALL support both manual copy-paste and screenshot ingestion methods

### Requirement 5: Update Docs Command

**User Story:** As a developer, I want AI-generated status updates ready for stakeholders, so that I can efficiently communicate progress without manual composition.

#### Acceptance Criteria

1. WHEN a user executes `tkt update` THEN THE Update_Generator SHALL review all task context (context.md, notes.md, communications/)
2. WHEN generating updates THEN THE Update_Generator SHALL produce a chronological structure of messages to send
3. WHEN generating updates THEN THE Update_Generator SHALL identify who to contact and why based on stakeholders.md and contacts.md
4. WHEN generating updates THEN THE Update_Generator SHALL identify what information is needed from stakeholders
5. WHEN generating updates THEN THE Update_Generator SHALL produce status updates formatted for copy/paste to email, Slack, and Teams
6. THE Update_Generator SHALL NOT automatically send communications to stakeholders; it SHALL only prepare content for user review

### Requirement 6: Knowledge Feedback Loop

**User Story:** As a developer, I want knowledge from completed tasks to automatically feed back into documentation, so that organizational learning accumulates over time.

#### Acceptance Criteria

1. WHEN a task is archived THEN THE Knowledge_Extractor SHALL read all communications, notes, and context from the completed task
2. WHEN extracting knowledge THEN THE Knowledge_Extractor SHALL summarize learnings, decisions, and gotchas
3. WHEN archiving a task THEN THE Handoff_Generator SHALL create a handoff.md document for future reference
4. WHEN knowledge is extracted THEN THE Wiki_Updater SHALL feed relevant knowledge back to docs/wiki/ with appropriate categorization
5. WHEN decisions are identified THEN THE ADR_Manager SHALL update docs/decisions/ with any ADRs that emerged from the task
6. WHEN updating documentation THEN THE Provenance_Tracker SHALL track "learned from TASK-XXXXX" attribution for all knowledge items

### Requirement 7: Conflict Detection

**User Story:** As a developer, I want conflicts with existing decisions and requirements detected early, so that I can resolve them before they become problems.

#### Acceptance Criteria

1. WHILE an AI coding session is active THEN THE Conflict_Detector SHALL monitor for conflicts with existing ADRs in docs/decisions/
2. WHILE an AI coding session is active THEN THE Conflict_Detector SHALL monitor for conflicts with previous task decisions
3. WHILE an AI coding session is active THEN THE Conflict_Detector SHALL monitor for conflicts with stakeholder requirements
4. WHEN a conflict is detected THEN THE Conflict_Detector SHALL surface the conflict to the user with context about the conflicting items
5. WHEN surfacing conflicts THEN THE Conflict_Detector SHALL provide recommendations for stakeholder resolution

### Requirement 8: Backlog Management

**User Story:** As a developer, I want a centralized backlog with rich metadata, so that I can prioritize and track all work items across projects.

#### Acceptance Criteria

1. THE Backlog_Manager SHALL maintain a backlog.yaml file with task entries containing: title, source, status, priority, owner, repo, branch, created date, tags, blocked_by, and related fields
2. WHEN a task status changes THEN THE Backlog_Manager SHALL update the corresponding entry in backlog.yaml
3. WHEN querying the backlog THEN THE Backlog_Manager SHALL support filtering by status, priority, owner, repo, and tags
4. WHEN a task has external references (Jira, Taskei, ServiceNow) THEN THE Backlog_Manager SHALL store and display the source reference
5. WHEN tasks have dependencies THEN THE Backlog_Manager SHALL track blocked_by and related task relationships
6. THE Backlog_Manager SHALL serialize and deserialize backlog.yaml preserving all task metadata

### Requirement 9: Multi-Repository Support

**User Story:** As a developer, I want to work across multiple repositories and platforms, so that I can manage tasks that span different codebases.

#### Acceptance Criteria

1. WHEN a task spans multiple repositories THEN THE Multi_Repo_Manager SHALL create worktrees in each relevant repository
2. THE Multi_Repo_Manager SHALL maintain unique task IDs across all repositories and platforms
3. THE Multi_Repo_Manager SHALL support GitHub, GitLab, and AWS CodeCommit (code.aws.dev) repository platforms
4. WHEN organizing repositories THEN THE Multi_Repo_Manager SHALL use the structure: repos/platform/org/repo/work/TASK-XXXXX
5. WHEN a cross-repo task is resumed THEN THE Multi_Repo_Manager SHALL restore context for all associated worktrees

### Requirement 10: Offline Mode

**User Story:** As a developer, I want core functionality to work offline, so that I can continue working without network connectivity.

#### Acceptance Criteria

1. WHILE offline THEN THE Task_Manager SHALL support task bootstrap and lifecycle management operations
2. WHILE offline THEN THE AI_Features SHALL be unavailable and THE System SHALL notify the user of limited functionality
3. WHEN connectivity is restored THEN THE Sync_Manager SHALL synchronize any pending changes
4. THE Offline_Detector SHALL accurately detect network connectivity status
5. WHILE offline THEN THE System SHALL queue operations requiring connectivity for later execution

### Requirement 11: Task Templates

**User Story:** As a developer, I want different task templates for different work types, so that each task starts with appropriate structure and context.

#### Acceptance Criteria

1. WHEN bootstrapping a task THEN THE Template_Manager SHALL apply templates based on task type: feat, bug, spike, refactor
2. THE Template_Manager SHALL support per-repository .taskrc configuration files specifying build commands, default reviewers, and conventions
3. WHEN a .taskrc exists in a repository THEN THE Template_Manager SHALL merge repository-specific settings with global defaults
4. WHEN no task type is specified THEN THE Template_Manager SHALL apply the default template
5. THE Template_Manager SHALL allow users to create and register custom task templates

### Requirement 12: Documentation Structure

**User Story:** As a developer, I want organized documentation that AI assistants can leverage, so that context is always available and up-to-date.

#### Acceptance Criteria

1. THE Documentation_System SHALL maintain docs/stakeholders.md containing people who care about outcomes (PMs, leads, customers)
2. THE Documentation_System SHALL maintain docs/contacts.md containing people who can help (SMEs, oncall, team aliases)
3. THE Documentation_System SHALL maintain docs/glossary.md containing team-specific terminology for AI context
4. THE Documentation_System SHALL maintain docs/decisions/ containing ADRs with task provenance
5. THE Documentation_System SHALL maintain docs/runbooks/ containing operational knowledge from incident tasks
6. THE Documentation_System SHALL maintain docs/wiki/ organized by topic/team/project for accumulated knowledge

### Requirement 13: Terminal and Tab Naming

**User Story:** As a developer, I want my terminal and editor tabs automatically named for the current task, so that I can easily identify which task I'm working on.

#### Acceptance Criteria

1. WHEN a task is started or resumed THEN THE Tab_Manager SHALL rename the terminal/tab to "TASK-XXXXX (branch-name)"
2. THE Tab_Manager SHALL support VS Code, Kiro, and standard terminal emulators
3. WHEN a task session ends THEN THE Tab_Manager SHALL restore the original tab name or set a neutral name
4. IF tab renaming fails THEN THE Tab_Manager SHALL log the error and continue without blocking task operations

### Requirement 14: Session Context Persistence

**User Story:** As a developer, I want AI session context to persist between sessions, so that I can seamlessly continue work without re-explaining context.

#### Acceptance Criteria

1. WHEN an AI session starts THEN THE Context_Manager SHALL load context.md, notes.md, and communications/ for the current task
2. WHILE an AI session is active THEN THE Context_Manager SHALL update context.md as work progresses
3. WHEN an AI session ends THEN THE Context_Manager SHALL persist the current context state
4. WHEN resuming a task THEN THE Context_Manager SHALL provide the AI assistant with full historical context
5. THE Context_Manager SHALL enable seamless handoffs between AI sessions by maintaining comprehensive context files

### Requirement 15: Configuration Management

**User Story:** As a developer, I want centralized configuration for the system, so that I can customize behavior to my workflow.

#### Acceptance Criteria

1. THE Configuration_Manager SHALL read global settings from .taskconfig file
2. THE Configuration_Manager SHALL support Taskfile.yaml for cross-platform CLI command definitions
3. WHEN configuration values conflict THEN THE Configuration_Manager SHALL apply precedence: repository .taskrc > global .taskconfig > defaults
4. THE Configuration_Manager SHALL validate configuration files and report errors with clear messages
5. IF a required configuration is missing THEN THE Configuration_Manager SHALL use sensible defaults and notify the user


### Requirement 16: AI Context File Generation

**User Story:** As a developer, I want an automatically generated and maintained AI context file (CLAUDE.md or kiro.md), so that AI coding assistants have immediate access to curated project knowledge.

#### Acceptance Criteria

1. THE AI_Context_Generator SHALL create and maintain a root-level AI context file (CLAUDE.md for Claude Code, kiro.md for Kiro) in the dev-brain directory
2. WHEN the AI context file is generated THEN THE AI_Context_Generator SHALL include: project overview, directory structure explanation, key conventions, glossary terms, and pointers to important documentation
3. WHEN wiki content is updated THEN THE AI_Context_Generator SHALL regenerate relevant sections of the AI context file to reflect current knowledge
4. WHEN ADRs are added or modified THEN THE AI_Context_Generator SHALL update the decisions summary section in the AI context file
5. THE AI_Context_Generator SHALL include links to stakeholders.md, contacts.md, and active task summaries
6. WHEN a user executes `tkt sync-context` THEN THE AI_Context_Generator SHALL regenerate the AI context file with the latest information from all sources

### Requirement 17: Task-Level Technical Design Document

**User Story:** As a developer, I want each task to have a technical design document that captures architecture and decisions, so that AI assistants and future developers understand the technical approach.

#### Acceptance Criteria

1. WHEN bootstrapping a new task THEN THE Bootstrap_System SHALL create a design.md file in tickets/TASK-XXXXX/
2. WHEN a task is started THEN THE Design_Document_Generator SHALL populate design.md with relevant context from docs/wiki/, related ADRs, and stakeholder requirements
3. WHILE working on a task THEN THE Design_Document_Generator SHALL update design.md with architecture diagrams, component descriptions, and technical decisions
4. WHEN communications contain technical decisions THEN THE Design_Document_Generator SHALL extract and add them to the task's design.md
5. THE Design_Document_Generator SHALL maintain design.md in a concise, accurate format using Mermaid diagrams for architecture visualization
6. WHEN a task is archived THEN THE Knowledge_Extractor SHALL use design.md as a primary source for extracting technical learnings and ADR candidates

### Requirement 18: External CLI Execution and Taskfile Integration

**User Story:** As a developer, I want adb to act as a command hub that can invoke external CLIs and Taskfile tasks, so that I have a single entry point for all my development workflows without switching between tools.

#### Acceptance Criteria

1. WHEN a user executes `adb exec <cli> [args...]` THEN THE CLI_Executor SHALL invoke the specified external CLI (e.g., aws, gh, git, docker, kubectl) as a subprocess, passing through all arguments and streaming stdout/stderr back to the user
2. WHEN a user executes `adb run <taskfile-task> [args...]` THEN THE Taskfile_Runner SHALL discover and execute the named task from the Taskfile.yaml in the current dev-brain directory or repository, passing through any arguments
3. WHEN executing an external CLI within a task context THEN THE CLI_Executor SHALL inject task-relevant environment variables (ADB_TASK_ID, ADB_BRANCH, ADB_WORKTREE_PATH, ADB_TICKET_PATH) into the subprocess environment
4. WHEN a Taskfile.yaml exists in the dev-brain root or current repository THEN THE Taskfile_Runner SHALL list available tasks via `adb run --list`
5. THE CLI_Executor SHALL support configuring CLI aliases in .taskconfig (e.g., mapping `adb aws` to `aws --profile myprofile --region us-east-1`)
6. WHEN an external CLI command fails THEN THE CLI_Executor SHALL capture the exit code and stderr, log the failure in the task's context if a task is active, and return the original exit code to the caller
7. THE CLI_Executor SHALL support piping and chaining of commands (e.g., `adb exec gh pr list | grep TASK-00042`)
8. WHEN a user executes `adb exec` without arguments THEN THE CLI_Executor SHALL display configured CLI aliases and available external tools
