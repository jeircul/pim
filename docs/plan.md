# Scope-Tree TUI Plan

## Objective

Build a deterministic, human-first activation experience that mirrors the Azure portal hierarchy in a terminal UI, while keeping automation support simple and explicit.

## Guiding Principles

1. **Role-first flow** – Users pick a role before navigating scopes; each role drives its own scope tree.
2. **Transparent data** – Display exactly what Azure returns. No synthetic scopes or hidden heuristics.
3. **Predictable navigation** – Model the scope hierarchy like a file tree with `h/j/k/l`, space to toggle, `/` to search, and `Enter` to confirm.
4. **Lazy loading** – Fetch subscriptions/resource groups only when their parent node expands to cut latency and API calls.
5. **Automation as opt-in** – Flags accept fully-qualified scope paths; CLI validates strictly and exits on mismatch.
6. **Lean code** – Separate TUI concerns from API plumbing; small, composable packages.

## User Flow

1. Launch `pim activate`.
2. **Role picker** (list + search). Multi-select allowed; `Enter` moves forward.
3. For each selected role, open a **scope tree view**:
   - Root shows management groups.
   - Expanding a management group (`l`) lists subscriptions.
   - Expanding a subscription lists resource groups.
   - Space toggles selection on any node (MG, subscription, RG).
   - Right status pane shows currently selected scopes.
4. `Enter` on the summary proceeds to justification + duration prompts.
5. Confirm activation summary, then submit requests sequentially.

## TUI Architecture

- **internal/ui/root** – manages app state (current role index, tree data, selection set).
- **internal/ui/tree** – reusable tree component with cursor movement, expand/collapse, and selection rendering.
- **internal/ui/prompt** – minimal inline prompts for justification/duration (no modal dialogs).
- Leverage `tcell` or `bubbletea` for terminal control; stick to ASCII to avoid wide-char issues.

## Data Layer Changes

- `pkg/azpim` gains:
  - `ListManagementGroups()` (if needed) or reuse known MG IDs.
  - `ListManagementGroupResourceGroups()` (already planned) for RG discovery without subscription access.
  - `ListSubscriptionResourceGroups()` remains but becomes a lazy fetch triggered by tree expansion.
- Cache responses per session to avoid repeated API calls when toggling nodes.

## Automation Mode

- Flags: `--role`, `--scope` (repeatable), `--time`, `--yes`.
- CLI verifies each scope path against the eligible list before submitting.
- Optional `--out json` to emit the activation payload for scripting.

## Open Questions

- Do we need inline filtering for large hierarchies (e.g., thousands of resource groups)? Potential solution: `:` command to filter nodes temporarily.
- Should we persist last-used selections/justification per role? Could live in `~/.config/pim/state.json`.
- Accessibility considerations (color vs monochrome). Default to monochrome-friendly palette.

## Next Steps

1. Prototype tree component with mocked data to validate navigation keys.
2. Integrate real `azpim` calls and ensure lazy loading is seamless.
3. Replace current prompt flow with the TUI + new automation path.
4. Write migration notes for `README.md` and update instructions to reflect v2 behavior.
