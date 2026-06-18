# Azure PIM REST API Reference

All requests use `https://management.azure.com` (ARM endpoint). API version for PIM
role-schedule endpoints: **`2020-10-01`**.

## Endpoints summary

| Purpose | Method | Path |
|---|---|---|
| Fetch eligible roles | GET | `/providers/Microsoft.Authorization/roleEligibilitySchedules` |
| Fetch active assignments | GET | `/providers/Microsoft.Authorization/roleAssignmentScheduleInstances` |
| Check active at scope | GET | `{scope}/providers/Microsoft.Authorization/roleAssignmentSchedules` |
| Activate / extend / deactivate | PUT | `{scope}/providers/Microsoft.Authorization/roleAssignmentScheduleRequests/{uuid}` |
| List eligible child resources | GET | `{mgScope}/providers/Microsoft.Authorization/eligibleChildResources?$getAllChildren=true` |
| List RGs for subscription | GET | `/subscriptions/{subID}/resourceGroups` |

---

## 1. Fetch eligible roles

```
GET https://management.azure.com/providers/Microsoft.Authorization/roleEligibilitySchedules
    ?api-version=2020-10-01
    &$filter=asTarget()
```

No `{scope}` prefix — this is a **tenant-wide** call. `asTarget()` filters to the
calling user's eligibilities only.

### Key response fields

```json
{
  "value": [{
    "id": "/providers/Microsoft.Management/managementGroups/{mgID}/providers/Microsoft.Authorization/roleEligibilitySchedules/{guid}",
    "properties": {
      "scope": "/providers/Microsoft.Management/managementGroups/{mgID}",
      "roleDefinitionId": "/providers/Microsoft.Management/managementGroups/{mgID}/providers/Microsoft.Authorization/roleDefinitions/{guid}",
      "expandedProperties": {
        "scope": { "displayName": "example-subscription" },
        "roleDefinition": { "displayName": "Reader" }
      }
    }
  }]
}
```

- `id` → stored as `Role.EligibilityScheduleID` — **full ARM resource path**, never strip
- `properties.scope` → stored as `Role.Scope`
- `properties.roleDefinitionId` → stored as `Role.RoleDefinitionID` — full scope-prefixed path, pass verbatim

> **Load-bearing sort:** `GetEligibleRoles` returns roles sorted by `(Scope, RoleName)`
> (`internal/azure/roles.go`). This sort is required for `autoAdvance` determinism —
> do not remove it. Selection logic in `rolelist.go` depends on stable order.

---

## 2. Fetch active assignments

```
GET https://management.azure.com/providers/Microsoft.Authorization/roleAssignmentScheduleInstances
    ?api-version=2020-10-01
    &$filter=asTarget()
```

Tenant-wide, `asTarget()` filter. Filter client-side by `properties.principalId` to
get only the calling user's assignments.

---

## 3. Check if a role is active at a scope

```
GET https://management.azure.com{scope}/providers/Microsoft.Authorization/roleAssignmentSchedules
    ?api-version=2020-10-01
    &$filter=principalId eq '{principalID}' and roleDefinitionId eq '{roleDefinitionID}'
```

Returns non-empty `value` array if active. HTTP 400/500 → treat as not active (swallow).

---

## 4. Activate / extend / deactivate a role

```
PUT https://management.azure.com{scopePath}/providers/Microsoft.Authorization/roleAssignmentScheduleRequests/{new-uuid}
    ?api-version=2020-10-01
```

`{new-uuid}` is a fresh `uuid.New().String()` per request.

### Request body

```json
{
  "properties": {
    "principalId": "{aad-object-id}",
    "roleDefinitionId": "{role.RoleDefinitionID}",
    "requestType": "SelfActivate",
    "justification": "optional text",
    "linkedRoleEligibilityScheduleId": "{role.EligibilityScheduleID}",
    "scheduleInfo": {
      "startDateTime": "2024-01-01T00:00:00Z",
      "expiration": {
        "type": "AfterDuration",
        "duration": "PT1H"
      }
    }
  }
}
```

| Field | Value | Notes |
|---|---|---|
| `principalId` | AAD object ID from `/me` | |
| `roleDefinitionId` | `role.RoleDefinitionID` verbatim | Scope-prefixed ARM path from eligibility response |
| `requestType` | `"SelfActivate"` / `"SelfExtend"` / `"SelfDeactivate"` | Extend if already active at scope |
| `linkedRoleEligibilityScheduleId` | `role.EligibilityScheduleID` verbatim | **Must be the full ARM path** — bare GUID does not work |
| `scheduleInfo` | Required for activate/extend | Omit for deactivate |
| `expiration.type` | `"AfterDuration"` | |
| `expiration.duration` | ISO 8601 duration e.g. `"PT1H30M"` | |

### Deactivate body (no scheduleInfo, no linkedRoleEligibilityScheduleId)

```json
{
  "properties": {
    "principalId": "{aad-object-id}",
    "roleDefinitionId": "{assignment.RoleDefinitionID}",
    "requestType": "SelfDeactivate"
  }
}
```

---

## 5. Scoped-down activation (JEA pattern)

A user with eligibility at a **management group** can activate at a narrower child scope
(subscription or resource group). The Azure portal calls this "Just-Enough-Access".

**How it works:**

- Use the child scope (`/subscriptions/{id}` or `/subscriptions/{id}/resourceGroups/{rg}`)
  as `{scopePath}` in the PUT URL.
- Keep `linkedRoleEligibilityScheduleId` = `role.EligibilityScheduleID` — the full ARM path
  of the **parent (MG-level)** eligibility schedule. Azure resolves the parent eligibility
  automatically from this ID, regardless of the URL scope.
- Keep `roleDefinitionId` = `role.RoleDefinitionID` verbatim from the eligibility response.

**Critical rule:** `linkedRoleEligibilityScheduleId` must always be the **full ARM resource
path** as returned by the `roleEligibilitySchedules` API. Stripping it to a bare GUID causes
HTTP 400 or HTTP 403 at the child scope.

> **Canonical key:** `linkedRoleEligibilityScheduleId` (`Role.EligibilityScheduleID`) is
> the globally-unique identifier for an eligibility. When it is known (stored in
> `Favorite.ScheduleID` or `RecentActivation.ScheduleID`), select the role by exact ID
> match and skip all name/scope heuristics. `pim search --output toml` always emits it.

### Why not re-query eligibility at the child scope?

`roleEligibilityScheduleInstances?$filter=asTarget()` at an RG or subscription scope returns
**only eligibilities defined directly at that scope** — inherited MG-level eligibilities are
not returned. Always use the tenant-wide `roleEligibilitySchedules?$filter=asTarget()` call
and pass the resulting `id` through unchanged.

> **Limitation**: RG-scope activation fails with HTTP 403 for MG-inherited eligibilities
> when the user lacks `resourceGroups/read` at the target RG — a chicken-and-egg since that
> permission is only granted after activation. The client automatically falls back to
> subscription scope. See section 8.

---

## 6. Eligible child resources

Used to enumerate scopes the user can narrow activation to.

```
GET https://management.azure.com{mgScope}/providers/Microsoft.Authorization/eligibleChildResources
    ?api-version=2020-10-01
    &$getAllChildren=true
```

**`$getAllChildren=true` is required.** Without it the API returns only direct children of
the management group. For deeply nested MG hierarchies this may return zero results even
when subscriptions exist at lower levels.

Handles pagination via `nextLink`. Confirmed live payload shape is flat items with
`id`, `name`, `type`, and sometimes `properties.displayName` (often empty). Match
`type` by case-insensitive substring, not suffix, because Azure returns bare singular
strings such as `"managementgroup"`, `"subscription"`, and `"resourcegroup"`:
- `type` contains `"subscription"` → subscription child scope
- `type` contains `"resourcegroup"` → resource group child scope
- `type` contains `"managementgroup"` → child management group

Do not use ARM path segment counts to infer direct vs nested children. Management group
IDs are flat (`/providers/Microsoft.Management/managementGroups/{id}`) regardless of
hierarchy depth, and subscription IDs are flat (`/subscriptions/{id}`) regardless of
which management group contains them.

**Child management groups must be treated as expandable nodes**, not discarded. A top-level MG
(e.g. `example-mg`) may return only child MGs with no subscriptions at the first level. Each child MG
is both selectable (the user can activate at the MG scope itself) and expandable (the user can
drill into it to find its own children, which may be further MGs or subscriptions).

`ListManagementGroupChildren` returns `([]ManagementGroup, []Subscription, error)`.
`ListManagementGroupSubscriptions` is a thin wrapper that discards the MG slice — use it only
for callers that genuinely need subscriptions only (e.g. headless mode).

An **empty response is valid** — it means the caller has no PIM-eligible child scopes under
that MG, not that the API failed.

**Do not fall back to the legacy `managementGroups/{id}/subscriptions` endpoint.** That
endpoint requires `Microsoft.Management/managementGroups/subscriptions/read`, which
PIM-eligible users typically do not have. The portal never uses it; `eligibleChildResources`
with `$getAllChildren=true` is the correct and sufficient path.

```json
{
  "value": [
    { "id": "/providers/Microsoft.Management/managementGroups/example-child-mg", "name": "example-child-mg", "type": "managementgroup", "properties": { "displayName": "" } },
    { "id": "/subscriptions/{subID}", "name": "{subID}", "type": "subscription", "properties": { "displayName": "" } },
    { "id": "/subscriptions/{subID}/resourceGroups/{rgName}", "name": "{rgName}", "type": "resourcegroup", "properties": { "displayName": "" } }
  ],
  "nextLink": "..."
}
```

---

## 7. Constants (internal/azure/client.go)

```go
apiVersion                        = "2020-10-01"   // all PIM role-schedule endpoints
eligibleChildResourcesAPIVersion  = "2020-10-01"   // eligibleChildResources
resourceGroupsAPIVersion          = "2021-04-01"   // ARM resourceGroups list
armEndpoint                       = "https://management.azure.com"
graphEndpoint                     = "https://graph.microsoft.com/v1.0"
```

---

## 8. RG-scope activation limitation and subscription fallback

Azure enforces `Microsoft.Resources/subscriptions/resourceGroups/read` at the target RG
scope **before** processing any PIM `roleAssignmentScheduleRequests` PUT. Since that
permission is only granted after activation, RG-scope activation of MG-inherited
eligibilities is impossible without a pre-existing assignment.

### Symptoms

| Request | Response | Meaning |
|---|---|---|
| PUT `{rgScope}/…/roleAssignmentScheduleRequests/{uuid}` | **HTTP 403 AuthorizationFailed** | Azure scope pre-check fails |
| GET `{rgScope}/…/roleAssignmentSchedules` | **HTTP 500** | No read access; `isRoleActiveAt` swallows this as "not active" — expected |
| PUT any scope | **HTTP 400 PendingRoleAssignmentRequest** | An activation request is already pending; treat as success-equivalent |

### Fallback pattern (implemented in `ActivateRole`)

When the RG-scope PUT returns HTTP 403 `AuthorizationFailed`, `ActivateRole` automatically
retries at the parent subscription scope:

```
1. Detect:  IsResourceGroupScope(scopePath) && isAuthorizationError(err)
2. Extract: subID := SubscriptionIDFromScope(rgScope)
3. Retry:   PUT /subscriptions/{subID}/providers/Microsoft.Authorization/
               roleAssignmentScheduleRequests/{new-uuid}
```

The retry uses the same request body — in particular the same MG-level
`linkedRoleEligibilityScheduleId`. Azure accepts this and activates the role at
subscription scope rather than the narrower RG.

### Confirmed via curl (example-subscription / my-rg)

```bash
# RG scope → 403
curl -X PUT ".../resourceGroups/my-rg/providers/Microsoft.Authorization/
  roleAssignmentScheduleRequests/{uuid}?api-version=2020-10-01" ...
# {"error":{"code":"AuthorizationFailed","message":"does not have authorization to
#  perform action 'Microsoft.Resources/subscriptions/resourceGroups/read' ..."}}
# HTTP 403

# Subscription scope → 201
curl -X PUT ".../subscriptions/30cfbf5f-.../providers/Microsoft.Authorization/
  roleAssignmentScheduleRequests/{uuid}?api-version=2020-10-01" ...
# {"properties":{"status":"Provisioned",...}}
# HTTP 201
```

### What was tried and did NOT work

| Approach | Result |
|---|---|
| Bare GUID for `linkedRoleEligibilityScheduleId` | HTTP 400 |
| Re-query `roleEligibilityScheduleInstances` at child scope | Returns empty — inherited MG eligibilities not visible |
| Keep full ARM path for `linkedRoleEligibilityScheduleId` (correct) but PUT at RG scope | HTTP 403 (Azure pre-check, not a request body issue) |

---

## 9. Large-tenant timeout & interactive-path rule

`ListAllSubscriptionsUnderMG` walks the MG tree via `eligibleChildResources?$getAllChildren=true`.
Each node gets `mgNodeTimeoutDefault = 15 s` (`internal/azure/discovery.go`).
On enterprise tenants with deep hierarchies, individual nodes consistently time out.

**Hard rule: MG expansion is a headless/background-only operation.**

| Path | Allowed? |
|---|---|
| `pim search` (headless) | ✓ — user expects latency; warnings go to stderr |
| TUI render / dashboard | ✗ — stalls UI thread |
| Favorite 1–9 shortcut | ✗ — stalls for 15s × N timed-out nodes |
| `--headless` activation | ✓ — blocks shell, user accepts latency |

The safe interactive contract: favorites carry pre-resolved `scope` + `schedule_id`.
No MG expansion is needed at activation time when these fields are set.

Override the per-node timeout with `PIM_MG_NODE_TIMEOUT` (e.g. `PIM_MG_NODE_TIMEOUT=45s`).
