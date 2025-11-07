# PIM – Azure PIM CLI

Minimal CLI for activating, deactivating, and inspecting Azure Privileged Identity Management (PIM) role assignments.

## Install

macOS / Linux:

```shell
curl -sSfL https://raw.githubusercontent.com/jeircul/pim/main/scripts/install.sh | bash
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/jeircul/pim/main/scripts/install.ps1 | iex
```

Pass a version (for example `v0.1.3`) as the final argument to pin a release. Ensure `~/.local/bin` (Unix) or `%LOCALAPPDATA%\Programs\pim` (Windows) is on `PATH`.

## Quick start

```shell
pim                 # guided menu with interactive prompts
pim activate -j "Deploy fix" --sub platform --auto
pim status          # list active assignments
pim deactivate      # stop an elevation early
pim help activate   # flag reference for scripted use
```

- Reuse existing `az login` / `Connect-AzAccount` sessions automatically.
- Filters (`--mg`, `--sub`, `--rg`, `--role`, `--scope`) can repeat and narrow the prompt.
- `--auto` applies matching hints without additional questions when only one target remains.

## Releases

Runtime version information comes from `git describe`. Tag and push to publish a release:

```shell
git tag v0.1.4
git push origin v0.1.4
```

For local checks, run `task build` to compile with the embedded version string. To package archives with GoReleaser, use `task release:snapshot` (dry run) or `task release:publish` (requires `GITHUB_TOKEN`).

## Development

```shell
task fmt      # go fmt ./...
task test     # go test ./...
task build    # go build ./...
task clean    # remove build artefacts
```

Report issues or ideas at [github.com/jeircul/pim](https://github.com/jeircul/pim).
