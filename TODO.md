# TODO

Items to resolve before merging `rewrite/v2` → `main`.

---

## Bugs

- [ ] **Justification selection broken** (`internal/tui/activate/options.go`)
  Recent justifications are displayed in the wizard options step but cannot be
  selected — the list items are not interactive. User must retype the justification
  every time.

---

## Testing

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
