# ⚡ pim — Azure PIM role elevation manager

Terminal UI for activating, deactivating, and inspecting Azure Privileged Identity Management (PIM) role assignments. Mirrors the Azure portal activation flow entirely in your terminal.

## ✨ Highlights

- 🖥️ Full-screen TUI — dashboard, activation wizard, status, deactivation, favorites management
- 🎯 Flags pre-fill wizard steps and auto-advance; `--headless` bypasses the TUI for scripting
- 🎨 Adaptive theme — works on light and dark terminals
- ⭐ Favorites with 1–9 number-key shortcuts for instant re-activation
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
pim version                  # print version
```

### 🏎️ Flag acceleration

Flags pre-fill wizard fields and skip steps when enough information is provided:

```sh
# Pre-filter the role list
pim activate --role Reader

# Jump straight to options step (role + scope already known)
pim activate --role Reader --scope /subscriptions/xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx

# Auto-submit with no TUI interaction
pim activate \
  --role Reader \
  --scope /subscriptions/xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx \
  --time 1h \
  --justification "Investigating alert" \
  --yes
```

### 🤖 Headless mode (scripting / CI)

```sh
# Activate — all four flags required
pim activate --headless \
  --role Reader \
  --scope /subscriptions/xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx \
  --time 1h \
  --justification "Deploy pipeline"

# Deactivate matching assignments
pim deactivate --headless --role Reader

# Status — JSON output
pim status --headless --output json
```

Exit code `0` on success, `1` on error.

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

`30m`, `1h`, `2h`, `1h30m`, `1.5h`. Range: 30 minutes – 8 hours in 30-minute increments.

## 🔐 Authentication

Uses the existing `az login` / `Connect-AzAccount` session automatically.  
Set `PIM_ALLOW_DEVICE_LOGIN=true` to allow interactive device code fallback when no cached credential is found.

## ⚙️ Configuration

State is stored in `~/.config/pim/`:

| File | Purpose |
|---|---|
| `config.toml` | Hand-editable preferences and favorites |
| `state.toml` | Auto-managed: recent justifications |

Example `config.toml`:

```toml
[preferences]
default_duration = "1h"

[[favorites]]
label = "Prod reader"
role  = "Reader"
scope = "/subscriptions/xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
duration = "1h"
key  = 1

[[favorites]]
label = "Dev owner"
role  = "Owner"
scope = "/subscriptions/yyyyyyyy-yyyy-yyyy-yyyy-yyyyyyyyyyyy"
duration = "2h"
key  = 2
```

## 🛠️ Development

```sh
task fmt      # go fmt ./...
task test     # go test ./...
task build    # go build ./...
task clean    # remove build artefacts
```

## 📤 Release

```sh
git tag v2.0.0
git push origin v2.0.0
```

GoReleaser builds cross-platform archives (linux, darwin, windows — amd64 + arm64) and attaches them to the GitHub release automatically.

## ⚠️ Disclaimer

Use at your own risk. The author is not responsible for any impact caused by its usage.  
AI tooling (GitHub Copilot) assisted with portions of the implementation, reviews, and documentation.
