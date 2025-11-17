---
description: PIM CLI tool - Azure elevation management
applyTo: '**'
---
Tool: Azure PIM role elevation CLI (Taskfile managed)
Tech: Go 1.23, Azure SDK, azidentity
Style: gofmt defaults (tabs), minimal code, no comments
Prefer: Idiomatic Go, error wrapping, immutability, least privilege
Never: Verbose output, new markdown docs, mutex unless required

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

Commands: status, activate, deactivate
Build: `task build`, Test: `task test`, Install: `task install`
