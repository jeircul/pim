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
# Activate — only --role is required; --time defaults to 1h, --scope to the
# eligibility scope, --justification may be empty
pim activate --headless \
  --role Reader \
  --scope /subscriptions/xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx \
  --time 1h \
  --justification "Deploy pipeline"

# Deactivate matching assignments (--role/--scope or --yes required to avoid
# accidentally deactivating everything; permanent and inherited assignments
# are skipped because they cannot be deactivated via PIM)
pim deactivate --headless --role Reader

# Status — JSON output
pim status --headless --output json
```

Exit code `0` on success, `1` on error, `130` on user cancel (Ctrl-C).

#### Matching policy for `--role` and `--scope`

Both flags resolve in two passes:

1. **Exact match** wins. `--scope my-rg` matches `my-rg` even if `my-rg-dev` also exists.
2. **Substring fallback** if no exact match. `--role admin` matches anything containing "admin".
3. **Ambiguity errors out.** If multiple values match by substring with no exact match, the command exits non-zero and lists the candidates instead of silently picking one.

ARM scope paths (`/subscriptions/...`) take precedence over display-name matching.

## 🐚 Shell completions

```sh
# bash — add to ~/.bashrc
source <(pim completion bash)

# zsh (simple) — add to ~/.zshrc
source <(pim completion zsh)

# zsh (fpath) — run once, then add the two lines below to ~/.zshrc
mkdir -p ~/.zfunc
pim completion zsh > ~/.zfunc/_pim
# fpath=(~/.zfunc $fpath)
# autoload -Uz compinit && compinit

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

### Prerequisites

- [Go](https://go.dev/dl/) 1.26.1+
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
