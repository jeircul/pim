# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Scope tree: `/` key opens a filter input to search scopes by name.
- Scope tree: viewport scrolling for large tenants with many scopes.
- Headless `--scope`: bare subscription GUID (e.g. `00000000-0000-0000-0000-000000000000`) expands to `/subscriptions/<guid>` automatically.
- Headless `--scope`: bare non-GUID token expands to the matching management group ARM path.
- Scope tree: child management group nodes are expandable; MG root is selectable even when child listing returns 403.

### Fixed

- MG root node was not selectable when listing its children returned 403.
- Scope tree rendered only 1 row before the first terminal resize event.
- `esc`/`enter` key events bubbled through the scope tree filter input to the wizard back/cancel handlers.

### Changed

- `ListManagementGroupChildren` now returns child management groups alongside subscriptions `([]ManagementGroup, []Subscription, error)`.
- `eligibleChildResources` always uses `$getAllChildren=true`; the legacy per-level endpoint is removed.
- `PendingRoleAssignmentRequest` (HTTP 400) is treated as success at all scopes — the role is already activating.
- GoReleaser config updated to `version: 2` with `snapshot.version_template`.

### Removed

- Legacy `managementGroups/{id}/subscriptions` endpoint; replaced by `$getAllChildren=true` on `eligibleChildResources`.
