# ⚡ pim — Azure PIM role elevation manager

Terminal UI for activating, deactivating, and inspecting Azure Privileged Identity Management (PIM) role assignments. Mirrors the Azure portal activation flow entirely in your terminal.

## ✨ Highlights

- 🖥️ Full-screen TUI — dashboard, activation wizard, status, deactivation, favorites management
- 🎯 Flags pre-fill wizard steps and auto-advance; `--headless` bypasses the TUI for scripting
- 🔭 Scope tree with `/` filter and viewport scrolling for large tenants
- 🎨 Adaptive theme — works on light and dark terminals
- ⭐ Favorites with 1–9 number-key shortcuts for instant re-activation
- 🕐 Recent elevations — `R` from the dashboard shows the last 10 successful activations; press Enter to re-activate with pre-filled fields
- 🔍 `pim search [query]` — find eligible subscriptions by name/GUID before activating
- 💾 TOML state persistence — remembers recent justifications and favorites across sessions
- 🐚 Shell completions for bash, zsh, and fish

## 📦 Install

macOS / Linux:

```sh
curl -sSfL https://raw.githubusercontent.com/jeircul/pim/main/scripts/install.sh | bash
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/jeircul/pim/main/scripts/install.ps1 | iex
```

`~/.local/bin` (Unix) or `%LOCALAPPDATA%\Programs\pim` (Windows) must be on `PATH`.

## 🚀 Quick start

```sh
pim                          # TUI dashboard — shows active elevations and favorites
pim activate                 # launch activation wizard from step 1
pim deactivate               # select and deactivate active elevations
pim status                   # view active and eligible roles
pim search my-subscription   # find eligible subscriptions matching "my-subscription"
pim search 00000000-...      # find by subscription GUID
pim version                  # print version
```

### 🏎️ Flag acceleration

Flags pre-fill wizard fields and skip steps when enough information is provided:

```sh
# Pre-filter the role list
pim activate --role Reader

# Jump straight to options step — scope matched by display name substring
pim activate --role Reader --scope my-subscription

# Bare subscription GUID expands to /subscriptions/<guid>
pim activate --role Reader --scope 00000000-0000-0000-0000-000000000000

# Auto-submit with no TUI interaction
pim activate \
  --role Reader \
  --scope my-subscription \
  --time 1h \
  --justification "Investigating alert" \
  --yes
```

Use the subscription GUID from `pim search` as `--scope` for headless activation. `pim search --output toml` emits the correct subscription ARM path for favorites.

### 🤖 Headless mode (scripting / CI)

```sh
# Activate — only --role is required; --time defaults to 1h
# --scope matches ARM path first, then display-name substring
# Bare GUIDs expand to /subscriptions/<guid>; bare non-GUID tokens expand to MG ARM paths
pim activate --headless \
  --role Reader \
  --scope my-subscription \
  --time 1h \
  --justification "Deploy pipeline"

# Deactivate by role name; --role/--scope or --yes required
# Permanent and inherited assignments are skipped automatically
pim deactivate --headless --role Reader

# Deactivate all eligible (use with care)
pim deactivate --headless --yes

# Status as JSON
pim status --headless --output json
```

Exit code `0` on success, `1` on error, `130` on user cancel (Ctrl-C).

### 🔍 pim search

Discover eligible subscriptions before activating. Use `--output toml` to generate a paste-ready `config.toml` favorite entry with the correct ARM scope already filled in.

```sh
pim search my-subscription          # filter by name
pim search 00000000-...             # filter by GUID
pim search --mg my-mgmt-group       # limit to a management group
pim search --output json            # machine-readable
pim search --output toml            # paste-ready [[favorites]] blocks
```

The `--output toml` format produces one `[[favorites]]` block per eligible role, with `scope` set to the subscription ARM path (`/subscriptions/<guid>`).

### 🔍 Matching policy for `--role` and `--scope`

Applies to both flag acceleration (TUI) and headless mode.

Both flags resolve in two passes:

1. **Exact match** wins. `--scope my-rg` matches `my-rg` even if `my-rg-dev` also exists.
2. **Substring fallback** if no exact match. `--scope prod` matches `my-prod-subscription` when it's the only match.
3. **Ambiguity errors out.** If multiple values match by substring with no exact match, the command exits non-zero and lists the candidates instead of silently picking one.

ARM scope paths (`/subscriptions/...`) take precedence over display-name matching. Bare subscription GUIDs (e.g. `00000000-0000-0000-0000-000000000000`) are automatically expanded to `/subscriptions/<guid>`; bare non-GUID tokens are expanded to the matching MG ARM path.

## 🐚 Shell completions

```sh
# bash — add to ~/.bashrc
source <(pim completion bash)

# zsh — add to ~/.zshrc
source <(pim completion zsh)

# fish
pim completion fish > ~/.config/fish/completions/pim.fish
```

## ⏱️ Duration format

The parser accepts any integer or decimal hours (`1h`, `1.5h` = 90 min), minutes (`30m`, `45m`), and mixed units (`1h30m`).

## 🔐 Authentication

Uses the existing `az login` / `Connect-AzAccount` session automatically.  
Set `PIM_ALLOW_DEVICE_LOGIN=true` (or `1` / `yes`) to allow interactive device code fallback when no cached credential is found.

## ⚙️ Configuration

State is stored in the platform config directory:

| Platform | Path |
|---|---|
| Linux / macOS | `$XDG_CONFIG_HOME/pim` (defaults to `~/.config/pim`) |
| Windows | `%APPDATA%\pim` (e.g. `C:\Users\<name>\AppData\Roaming\pim`) |

| File | Purpose |
|---|---|
| `config.toml` | Hand-editable preferences and favorites |
| `state.toml` | Auto-managed: recent justifications and recent activations |

Example `config.toml`:

```toml
[preferences]
default_duration = "2h"

[[favorites]]
label         = "Contributor @ my-subscription"
role          = "Contributor"
scope         = "/subscriptions/00000000-0000-0000-0000-000000000000"
duration      = "1h"
justification = "Daily access"
key           = 1

[[favorites]]
label         = "Reader @ my-subscription"
role          = "Reader"
scope         = "00000000-0000-0000-0000-000000000000"
duration      = "2h"
justification = "Read-only investigation"
key           = 2
```

**Which form to use:** Run `pim search --output toml` to get a paste-ready `[[favorites]]` block for any eligible role. The `scope` field is always a subscription ARM path (`/subscriptions/<guid>`) — safe to use verbatim. Fill in `justification`, `duration`, and `key`, then paste into `config.toml`.

`label` is required. When `role`, `scope`, `duration`, and `justification` are all set, pressing the shortcut key activates immediately with no prompts and returns to the dashboard with a result notice. If any field is missing the shortcut shows an error notice — open the favorite in the favorites editor (`f`) and activate from there; the wizard will stop at the first missing field.

### Recent activations

Press `R` from the dashboard to open the recent activations screen. It shows the last 10 **successful** activations (role, scope, duration, time ago, justification). Recent activations store the original eligibility scope so re-activation is as precise as using an MG ARM path directly in a favorite. Press `Enter` on any row to open the activation wizard pre-filled with those details. Press `esc` or `q` to return to the dashboard.

## 🛠️ Development

### Prerequisites

- [Go](https://go.dev/dl/) 1.26+
- [Task](https://taskfile.dev/) (task runner)
- [GoReleaser](https://goreleaser.com/) (releases only)

The quickest way to install all tools at once (Linux, macOS, WSL):

```sh
# Install mise (https://mise.jdx.dev/)
curl https://mise.run | sh

# Install all declared tools from .mise.toml
mise install
```

Or install each tool manually via their respective docs.

### Common tasks

```sh
task build    # build binary for current platform
task test     # go test -race ./...
task fmt      # go fmt ./...
task install  # build + install to ~/.local/bin
task clean    # remove build artefacts
```

## 📤 Release

```sh
git tag vX.Y.Z
git push origin vX.Y.Z
```

GoReleaser builds cross-platform archives (linux, darwin, windows — amd64 + arm64) and attaches them to the GitHub release automatically.

## ⚠️ Disclaimer

Use at your own risk. The author is not responsible for any impact caused by its usage.  
AI tooling (GitHub Copilot) assisted with portions of the implementation, reviews, and documentation.
