# Email Processing

## Overview

The email processing system is a personal Gmail knowledge hub that syncs emails into git as structured markdown, applies AI-powered classification, summarization, action extraction, and digest generation. It runs as a fully automated CI pipeline every 6 hours (TASK-00021).

The system uses `gog` (Google CLI tool v0.9.0) for Gmail and Contacts API access via OAuth, with all processing handled locally. Emails are stored as markdown files with YAML frontmatter containing metadata fields: `id`, `from`, `to`, `cc`, `date`, `subject`, `labels`, `category`, `priority`, `summary`, and `processed`.

## Key Decisions

- **Email format**: Markdown with YAML frontmatter -- human-readable, git-diff friendly, grep-searchable (K-00001)
- **Sync strategy**: Local-first via `gog` CLI with OAuth tokens in the system keyring (K-00002)
- **HTML-to-text**: Conversion via `pandoc` (K-00003)
- **Search granularity**: Message-level search via `gog gmail messages search` (not thread-level) for accurate per-message files (K-00004)
- **Idempotency**: Filename-based dedup using message ID as filename, plus `processed: true` frontmatter flag (K-00005)
- **Sync state**: Tracked in `config/sync-state.yaml` with last sync timestamp and history ID (K-00006)
- **AI model**: Haiku 4.5 for all processing -- Sonnet returns a 400 billing header error on Bedrock (K-00007)
- **Structured output**: All AI calls use `--output-format json --json-schema` to force deterministic JSON responses; avoids model confusion from project context in `claude -p` (K-00008)
- **Batch size**: 15 emails per AI call for cost efficiency (K-00009)
- **Contacts merge**: Match by slug or email, preserve AI fields, add Google fields (K-00011)
- **Label indexes**: Regenerated from scratch each run (no incremental updates) for simplicity and correctness (K-00012)

## Learnings

- The `processed: true` frontmatter flag is the primary idempotency mechanism for the `process-emails.sh` script. Always check this field before re-processing.
- Structured JSON output mode is critical for reliability when calling Claude from shell scripts. Without it, the model may produce free-form text influenced by project context files.
- The JSON envelope returned by `claude -p --output-format json` contains `.structured_output` and `.result` fields; parse via the `ai_call()` helper in `ai.sh`.

## Related

- ADR-0001: Observability Infrastructure (structured logging patterns apply to email processing events)
- Glossary terms: `gog`, `email frontmatter`, `processed email`, `structured output`, `JSON envelope`, `contact slug`

---
*Sources: TASK-00021*
