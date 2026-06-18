# pim search — MG Expansion & TOML Output Reference

This document covers the design of `pim search`, the management group expansion
pipeline, and the `--output toml` guarantees. Read this before extending any
search, discovery, or TOML-output code path.

## Hard constraints

### MG/subscription ARM paths are flat

ARM scope paths for management groups and subscriptions are structurally
unrelated strings:

- MG: `/providers/Microsoft.Management/managementGroups/{id}`
- Subscription: `/subscriptions/{guid}`

`ScopeIsChildOf(child, parent string)` uses `strings.HasPrefix` — it **cannot
cross the MG/subscription boundary**. A subscription is semantically a child of
an MG but never a string-path child. Never attempt to infer MG→subscription
parentage from scope strings alone.

### `$getAllChildren=true` is headless-only

`ListAllSubscriptionsUnderMG` calls `eligibleChildResources?$getAllChildren=true`
on each MG node. Per-node timeout is `mgNodeTimeoutDefault = 15s`
(`internal/azure/discovery.go`). On enterprise tenants with deep hierarchies,
this fans out to dozens of nodes and consistently exceeds 15s per stalled node.

**Never call `ListAllSubscriptionsUnderMG` synchronously in an interactive path**
(TUI render, dashboard shortcut, favorite activation). It will stall enterprise
tenants. The safe interactive-path contract is: favorites carry pre-resolved
`scope` + `schedule_id`; expansion runs only in `pim search` (headless).

Inaccessible MG nodes are skipped with a stderr warning — not fatal. The
command completes with partial results.

### `EligibilityScheduleID` is the canonical activation key

`Role.EligibilityScheduleID` is the full ARM resource path of the eligibility
schedule as returned by `GetEligibleRoles`. It is globally unique per
(principal, role definition, eligibility scope) and is what `ActivateRole` sends
in `linkedRoleEligibilityScheduleID`. When it is known, select roles by exact ID
match — never by name/scope heuristics.

## buildSearchHits pipeline

```
GetEligibleRoles()
    │
    ▼
buildSearchHits()          ← MG expansion happens here
    │  returns ([]SearchHit, subRoleMap, error)
    │
    ├── filterSearchHits()  ← query string filter
    ├── filterHitsByMG()    ← --mg filter
    │
    ├── jsonOut()           ← --output json
    ├── tomlFromHits()      ← --output toml  (uses subRoleMap)
    └── tabwriter           ← --output table (default)
```

### SearchHit shape

```go
type SearchHit struct {
    SubscriptionID   string   // resolved subscription GUID
    DisplayName      string   // subscription display name
    ManagementGroup  string   // direct physical parent MG ID (may differ from eligibility MG)
    EligibilityScope string   // first-writer MG ARM path (lossy — one per hit)
    EligibleRoles    []string // role names only — use subRoleMap for full Role objects
}
```

`EligibleRoles` carries names only. The associated `azure.Role` objects (with
`EligibilityScheduleID` and `Scope`) live in `subRoleMap`.

### subRoleMap contract

`buildSearchHits` returns a `map[string]map[string]azure.Role` alongside
`[]SearchHit`:

- Outer key: `strings.ToLower(subscriptionID)`
- Inner key: `strings.ToLower(roleName)`
- Value: the exact `azure.Role` that granted this role to this subscription,
  captured **at expansion time** inside the `add()` closure

First-writer-wins when the same (subscription, roleName) pair is reachable via
multiple eligibilities (e.g. both a sub-direct role and an MG-inherited role).

**This is the canonical way to recover the granting `azure.Role` for any
(subscription, roleName) pair. Never reconstruct it from `SearchHit` fields
downstream — see anti-patterns.md.**

## tomlFromHits contract

`tomlFromHits(hits []SearchHit, subRoleMap map[string]map[string]azure.Role, out io.Writer) error`

For each `SearchHit`, iterates `h.EligibleRoles` and looks up the granting
`azure.Role` via `subRoleMap[subKey][roleName]`. Emits one `[[favorites]]` block
per (subscription, role) pair.

### What --output toml guarantees

Every emitted block has:

| Field | Value | Source |
|---|---|---|
| `scope` | `/subscriptions/<guid>` | activation target (PUT URL) |
| `eligibility_scope` | MG ARM path | MG-inherited roles only; from `role.Scope` |
| `schedule_id` | full eligibility ARM path | `role.EligibilityScheduleID` |

Users fill in only `duration`, `justification`, and `key`. Never write
`schedule_id` or `eligibility_scope` by hand.

`scope` and `schedule_id` are always both present even for sub-direct roles
where they reference the same subscription — they serve different purposes:
`scope` is the PUT URL target; `schedule_id` is the request body eligibility
reference.

### Role object selection precedence (tomlFromHits)

For each (subscription, roleName) pair:
1. `subRoleMap[subID][roleName]` — direct lookup, no fallback needed

If the lookup misses (role in `EligibleRoles` but absent from `subRoleMap` —
should not happen in normal operation): skip the block rather than guessing.

## MG expansion internals

`ListAllSubscriptionsUnderMG` does a bounded parallel BFS:

- 8 concurrent workers (`const workers = 8`)
- 15s per-node timeout (`mgNodeTimeoutDefault`, overridable via `PIM_MG_NODE_TIMEOUT`)
- Adaptive clamp: `min(mgNodeTimeoutDefault, remaining parent deadline)`
- Results: `(subs []Subscription, parents map[subID]mgID, warnings []string, err error)`
- `parents` maps each subscription to its **direct physical parent MG ID** — this
  may differ from the eligibility MG when eligibility is granted at an ancestor

The `parents` map uses lexically-smallest MG ID per subscription for
deterministic output on diamond topologies.

## Adding a new --output mode

1. Add a constant to `internal/app/config.go` `OutputFormat`
2. Add a case in `runSearchWithErr` after the JSON branch, before the table branch
3. Consume `hits` and `subRoleMap` — both are available at that point
4. Do not re-expand MGs; do not call `ListAllSubscriptionsUnderMG`
5. Add a test in `search_test.go` using `searchMock` with `mgSubs` populated
