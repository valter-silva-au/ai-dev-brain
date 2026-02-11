# Documentation

Project documentation, knowledge base, and decision records. This directory is the single source of truth for organizational knowledge accumulated across tasks.

## Structure

```
docs/
├── stakeholders.md          # Outcome owners and key decision makers
├── contacts.md              # Subject matter experts and external contacts
├── glossary.md              # Project terms, domain terms, acronyms
├── wiki/                    # Extracted knowledge from completed tasks
├── decisions/               # Architecture Decision Records (ADRs)
│   └── ADR-TEMPLATE.md      # Template for new ADRs
└── runbooks/                # Operational procedures and guides
```

## Conventions

- Use ADRs to record significant technical decisions -- see `decisions/ADR-TEMPLATE.md`
- Update the glossary as new terms emerge in communications and design discussions
- Extract learnings from completed tasks into the wiki during archival
- Keep stakeholder and contact information current as team members change
- Runbooks should be actionable step-by-step procedures, not explanations

## Knowledge Flow

When tasks are archived with `adb archive`, learnings and decisions are extracted and fed into:
- `wiki/` -- general knowledge and best practices
- `decisions/` -- Architecture Decision Records
- `runbooks/` -- operational procedures
