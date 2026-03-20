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
| List eligible child resources | GET | `{mgScope}/providers/Microsoft.Authorization/eligibleChildResources` |
| List MG subscriptions (fallback) | GET | `/providers/Microsoft.Management/managementGroups/{mgID}/subscriptions` |
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
        "scope": { "displayName": "Omnia-Classic-QA" },
        "roleDefinition": { "displayName": "Reader" }
      }
    }
  }]
}
```

- `id` → stored as `Role.EligibilityScheduleID` — **full ARM resource path**, never strip
- `properties.scope` → stored as `Role.Scope`
- `properties.roleDefinitionId` → stored as `Role.RoleDefinitionID` — full scope-prefixed path, pass verbatim

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
HTTP 403 `AuthorizationFailed` at the child scope.

### Why not re-query eligibility at the child scope?

`roleEligibilityScheduleInstances?$filter=asTarget()` at an RG or subscription scope returns
**only eligibilities defined directly at that scope** — inherited MG-level eligibilities are
not returned. Always use the tenant-wide `roleEligibilitySchedules?$filter=asTarget()` call
and pass the resulting `id` through unchanged.

---

## 6. Eligible child resources

Used to enumerate scopes the user can narrow activation to.

```
GET https://management.azure.com{mgScope}/providers/Microsoft.Authorization/eligibleChildResources
    ?api-version=2020-10-01
    &getAllChildren=true
```

Handles pagination via `nextLink`. Returns both subscriptions (`type` contains `"subscription"`)
and resource groups (`type` contains `"resourcegroup"`).

```json
{
  "value": [
    { "id": "/subscriptions/{subID}/resourceGroups/{rgName}", "name": "{rgName}", "type": "resourcegroup" }
  ],
  "nextLink": "..."
}
```

---

## 7. Constants (internal/azure/client.go)

```go
apiVersion                             = "2020-10-01"   // all PIM role-schedule endpoints
managementGroupSubscriptionsAPIVersion = "2023-04-01"   // MG subscriptions fallback
eligibleChildResourcesAPIVersion       = "2020-10-01"   // eligibleChildResources
resourceGroupsAPIVersion               = "2021-04-01"   // ARM resourceGroups list
armEndpoint                            = "https://management.azure.com"
graphEndpoint                          = "https://graph.microsoft.com/v1.0"
```
