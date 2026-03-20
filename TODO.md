# TODO

Items to resolve before merging `rewrite/v2` → `main`.

---

## Bugs

- [x] **Early keypress quits app** (`internal/tui/app.go`, `internal/tui/dashboard/dashboard.go`)
  Pressing `a` or `D` before `GetCurrentUser()` resolves killed the app with an error.
  Fixed: `userReady` guard swallows those actions until identity is resolved; auth errors
  are surfaced in the dashboard instead of silently dropped.

- [x] **Justification selection broken** (`internal/tui/activate/options.go`)
  `↑`/`↓` navigation in the recent-justifications list only worked after tabbing into
  the text field. Fixed: `↑`/`↓` now works from the duration grid too and auto-switches
  focus to the justification field on first press.

- [ ] **Deactivation summary shows all roles, not just selected ones** (`internal/tui/deactivate/deactivate.go`)
  `collectResults()` iterates over all items regardless of the `selected` field, so every
  unselected assignment appears as `"deactivated: ..."` in the exit summary even though it
  was never touched. Fix: skip items where `!it.selected` in `collectResults`.

- [ ] **Headless deactivate has no safety gate** (`internal/headless/run.go`)
  `pim deactivate --headless` with no `--role`/`--scope` flags calls `filterAssignments`
  which returns all active assignments when both filter slices are empty — silently
  deactivating every active PIM elevation with no confirmation. Unlike activation,
  deactivation has zero required flags and no `--yes` check. Fix: either require at least
  one filter flag, or honour `--yes` as an explicit "deactivate all" gate.

### Headless path (`internal/headless/run.go`)

Filter logic is tested (`filterRoles`, `filterAssignments`, `matchesAny`), but
`runActivate`, `runDeactivate`, and `runStatus` have no test coverage.

To test them, define a `ClientAPI` interface at the consumer (per conventions —
interfaces at consumer, not implementer) and inject a mock.

- [ ] **`runActivate`**
  - Missing required flags (`--role`, `--scope`, `--time`, `--justification`) → error
  - No matching roles for given filters → error
  - Successful activation → prints confirmation, saves justification to state
  - Partial failure (one role fails) → prints errors to stderr, returns last error
  - RG-scoped target with 403 fallback → succeeds at subscription scope

- [ ] **`runDeactivate`**
  - No matching active assignments → prints "No matching active assignments."
  - Successful deactivation → prints confirmation per role
  - Deactivation error → prints to stderr, continues to next assignment

- [ ] **`runStatus`**
  - No active assignments → prints "No active PIM elevations."
  - JSON output mode (`--output json`) → valid JSON array
  - Table output → correct tab-aligned columns

### Manual smoke test

```bash
./pim activate --headless \
  --role Reader \
  --scope /subscriptions/30cfbf5f-c7f9-4f5b-8774-5ef62ed2f22d \
  --time 1h \
  --justification "smoke test"
```

Verify: activates at subscription scope (due to RG fallback if RG was selected),
prints `Activated: Reader @ /subscriptions/... for 1h`, exit 0.

---

## Low priority

- [ ] **Bug #8**: Context stored at construction in `internal/azure/client.go` —
  should use request-scoped context passed per-call rather than stored at `NewClient` time.

- [ ] **Dead code**: `ListSubscriptionResourceGroups` is no longer called from the TUI
  path (replaced by `eligibleChildResources` API). Audit and remove if unused.
