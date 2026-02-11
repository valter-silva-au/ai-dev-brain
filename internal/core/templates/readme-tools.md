# Tools

Project-specific scripts, binaries, and automation utilities. Keep tooling here rather than requiring global installs so that the project is self-contained and reproducible.

## Suggested Organization

```
tools/
├── scripts/                 # Shell scripts, automation
├── hooks/                   # Git hooks, CI hooks
└── bin/                     # Local binaries, tools
```

## Guidelines

- Document each tool's purpose and usage with a comment header or accompanying README
- Version-control scripts alongside the project
- Prefer portable scripts (POSIX sh) over bash-specific syntax when possible
- Keep CI/CD pipeline scripts here rather than inline in config files
