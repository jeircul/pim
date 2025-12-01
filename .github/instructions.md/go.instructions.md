pkg/
tests/
# GitHub Copilot Instructions

These guidelines tailor Copilot’s suggestions to the PIM CLI codebase so generated code matches our conventions and tooling.

## 🧠 Context

- **Project**: Azure PIM elevation CLI (`pim`)
- **Language**: Go 1.23
- **Key libs**: `azidentity`, custom `pkg/azpim`, `github.com/lithammer/fuzzysearch/fuzzy`
- **Structure**:
  - `main.go`: entrypoint, version, error handling
  - `internal/cli`: flag parsing, prompts, handlers
  - `pkg/azpim`: Azure client, REST helpers, types

## 🔧 General Guidance

- Follow Effective Go; format with `gofmt`.
- Keep `main.go` minimal: parse args, wire dependencies, call CLI handlers.
- Propagate `context.Context` (default timeout 2m) through handlers and Azure client calls.
- Prefer small, focused functions and early returns.
- Wrap errors with context using `fmt.Errorf("action: %w", err)`.
- Avoid new top-level globals; inject dependencies via params/structs.
- No verbose logging—print concise status lines matching current UX.

## 🧶 Patterns to Use

- Accept interfaces (e.g., `azpim.Client`) and return structs for data.
- Keep config structs immutable once validated; expose helper methods (e.g., `ModeLabel`, `HasFilters`).
- For prompts, reuse helpers in `internal/cli/prompt.go` and keep user messaging consistent.
- When filtering slices, build new slices; never mutate arguments in place.
- Validate user input early (`ActivateConfig.Validate`, duration parsing, etc.).

## 🚫 Avoid

- Adding new markdown/docs unless explicitly requested.
- Introducing logging libraries, Cobra, or HTTP servers.
- Long anonymous functions; prefer named helpers within package.
- Global mutable state, mutexes, or channels unless required.
- Silently swallowing errors—bubble up with context.

## 🧪 Testing

- Use Go’s `testing` package with table-driven tests.
- Keep CLI tests under `internal/cli`, client tests under `pkg/azpim`.
- When mocking, prefer lightweight fakes/interfaces defined near usage.

## 💡 Example Prompts

- “Add a prompt helper for confirming multiple role activations.”
- “Parse a duration flag supporting 30m increments.”
- “Unit test activation filter logic with subscription/resource-group hints.”

## 🔁 Workflow

- Run `task fmt` and `task test` (or `go test ./...`) before committing.
- Keep diffs focused; avoid touching unrelated files.
- Mention any manual test commands in PR descriptions.