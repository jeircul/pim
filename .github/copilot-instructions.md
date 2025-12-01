---
description: PIM CLI tool - Azure elevation management
applyTo: '**'
---
Tool: Azure PIM role elevation CLI (Taskfile managed)
Tech: Go 1.23, Azure SDK, azidentity
Style: gofmt defaults (tabs), minimal code, no comments
Prefer: Idiomatic Go, error wrapping, immutability, least privilege
Never: Verbose output, new markdown docs, mutex unless required, inline comments/TODOs

Structure:
- `main.go`: entry, version, error handling
- `internal/cli/`: command parsing, prompts, handlers  
- `pkg/azpim/`: Azure PIM client, REST API, types

Patterns:
- Accept interfaces, return structs
- Error wrapping: `fmt.Errorf("context: %w", err)`
- Early returns, guard clauses over nesting
- Immutable config structs, validation methods
- Context propagation (2min timeout default)
- Slice/map operations return new, never mutate
- String ops: `strings.ToLower()`, `strings.Contains()`
- HTTP timeout: 30s

UX guardrails:
- `activate` flow is confirmation-first; `--yes/-y` is the only automation bypass.
- Durations accept 30-minute increments via parsed string flags (e.g., `30m`, `1.5h`).
- Prompts live in `internal/cli/prompt.go`; keep wording consistent and concise.

Discipline:
- Keep the tree legacy-free—delete obsolete flags, code paths, and docs immediately after behavior changes.
- Code should be self-documenting; do not add commentary while developing.

Commands: status, activate, deactivate
Tasks: `task build`, `task test`, `task install`
