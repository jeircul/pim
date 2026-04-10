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

- [x] **Deactivation summary shows all roles, not just selected ones** (`internal/tui/deactivate/deactivate.go`)
  `collectResults()` iterated over all items regardless of `selected`; fixed by skipping
  items where `!it.selected` in `collectResults`.

- [x] **Headless deactivate has no safety gate** (`internal/headless/run.go`)
  `pim deactivate --headless` with no `--role`/`--scope` flags silently deactivated
  everything. Fixed: guard at top of `runDeactivate` requires at least one filter flag or
  `--yes` to deactivate all.

### Headless path (`internal/headless/run.go`)

- [x] **`ClientAPI` interface + mock tests** — `runActivate`, `runDeactivate`, and `runStatus`
  now have full table-driven test coverage via `ClientAPI` interface injection.
  See `internal/headless/run_test.go`.

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

- [x] **Dead code**: `ListSubscriptionResourceGroups` removed (was no longer called from
  the TUI path; replaced by `eligibleChildResources` API). Constant `resourceGroupsAPIVersion`
  also removed.
