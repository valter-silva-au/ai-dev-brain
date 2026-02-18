# Knowledge and Feedback Loop System

## Overview

The knowledge and feedback loop system addresses a core gap in AI Dev Brain: per-task knowledge becomes effectively invisible once tasks are archived. The system provides cross-task, long-term knowledge persistence as a queryable memory, combined with a closed-loop architecture that ingests context from multiple channels, routes work to AI agents with accumulated knowledge, produces output back to those channels, and persists knowledge across task lifecycles (TASK-00034).

The system has three major components:

1. **Knowledge Store** -- Long-term, queryable storage for decisions, learnings, patterns, gotchas, and relationships extracted from tasks
2. **Channel Adapters** -- Input/output connectors for email, messaging, tickets, documents, socials, events, and career channels
3. **Feedback Loop Orchestrator** -- Coordinates fetch, classify, route, process, deliver, and learn cycles

## Key Decisions

- **File-based YAML for knowledge store**: Consistent with adb's existing file-based storage patterns. Knowledge entries, topics, entities, and timeline are stored as YAML files under `docs/knowledge/` (K-00025).
- **Knowledge IDs use K-XXXXX format**: Sequential IDs following the same pattern as task IDs (TASK-XXXXX), providing a consistent identifier scheme across the system (K-00026).
- **Channel adapters implement a common Go interface**: All channel types (file, email, messaging, etc.) implement the same adapter interface, enabling uniform fetch/send operations regardless of the underlying transport (K-00027).
- **Git-repo adapter is the first channel type**: The file-based channel adapter was implemented first because it was proven by the gmail email sync pattern and requires no external dependencies (K-00028).
- **Topics are user-managed, AI-suggested**: The system suggests topic categorizations but requires user approval, preventing uncontrolled topic explosion that would degrade knowledge findability (K-00029).
- **Three-phase implementation**: Knowledge Store first, then Channel Adapters, then Feedback Loop. Each phase builds on the previous one and can deliver value independently (K-00030).

## Learnings

- Cross-task knowledge search ("what decisions were made about X across all tasks?") requires a dedicated index, not just per-task files. The knowledge store's `index.yaml` and `topics.yaml` serve this purpose.
- The feedback loop pattern (fetch -> classify -> route -> process -> deliver -> learn) maps cleanly to adb's existing architecture. Each step corresponds to an existing or new interface method.
- Topic explosion is a real risk in knowledge management systems. The user-managed, AI-suggested approach is a pragmatic compromise that keeps the topic graph useful.

## CLI Commands

- `adb knowledge query <term>` -- Search knowledge by keyword, topic, entity, or tag
- `adb knowledge add <summary>` -- Manually add a knowledge entry
- `adb knowledge topics` -- List topics and their relationships
- `adb knowledge timeline` -- Show chronological knowledge trail
- `adb channel list` -- List registered channel adapters
- `adb channel inbox [adapter]` -- Show pending items
- `adb channel send <adapter> <dest> <subject> <content>` -- Send output
- `adb loop [--dry-run] [--channel name]` -- Execute a feedback loop cycle

---
*Sources: TASK-00034*
