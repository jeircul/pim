# ⚡️ PIM – Azure PIM CLI

Small, friendly CLI for activating, deactivating, and inspecting Azure Privileged Identity Management (PIM) role assignments.

## ✨ Highlights

- 🔐 Guided menu when you launch `pim` with no arguments
- 🎯 Fast flag flow for scripts (`pim activate -j "Patch" --sub prod --auto`)
- 🧭 Scoped activations for management groups, subscriptions, and resource groups
- 🔁 Quick status and deactivate commands for active elevations

## 📦 Install

macOS / Linux:

```shell
curl -sSfL https://raw.githubusercontent.com/jeircul/pim/main/scripts/install.sh | bash
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/jeircul/pim/main/scripts/install.ps1 | iex
```

Add a version (for example `v0.1.3`) as the final argument to pin a release. Make sure `~/.local/bin` (Unix) or `%LOCALAPPDATA%\Programs\pim` (Windows) is on `PATH`.

## 🚀 Quick Start

```shell
pim                     # guided prompts with search and filtering
pim activate -j "Deploy" --sub platform --rg app --auto
pim status              # show active assignments
pim deactivate          # stop an elevation early
pim help activate       # discover all flags and options
```

- Reuses existing `az login` / `Connect-AzAccount` sessions (enable `PIM_ALLOW_DEVICE_LOGIN=true` to allow device code fallback).
- Filters (`--mg`, `--sub`, `--rg`, `--role`, `--scope`) can repeat and narrow the interactive list.
- `--auto` applies matching hints without further prompts when a single target remains.

## 📤 Releases

Runtime version details come from `git describe`. Publish a new release by tagging and pushing:

```shell
git tag v0.1.4
git push origin v0.1.4
```

A GitHub Actions workflow triggers GoReleaser to build and attach platform archives automatically.

## 🛠️ Development

```shell
task fmt      # go fmt ./...
task test     # go test ./...
task build    # go build ./...
task clean    # remove build artefacts
```

## ⚠️ Disclaimer & Attribution

- Use this tool at your own risk; the author is not responsible for any impact caused by its usage.
- Artificial intelligence (GitHub Copilot & friends) assisted with portions of the implementation, reviews, and documentation.

## 🙌 Get Involved

Report issues, share ideas, or follow releases at [github.com/jeircul/pim](https://github.com/jeircul/pim).
