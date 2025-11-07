# ⚡️ PIM - Azure PIM CLI

Opinionated command-line tool for activating, deactivating, and checking Azure Privileged Identity Management (PIM) role assignments.

## ✨ Features

- 🔐 Activate eligible Azure role assignments with justifications and custom duration.
- 🔄 Deactivate active assignments in seconds.
- 👀 Inspect current elevations with `--status`.
- 💻 Works on macOS, Linux, and Windows (amd64 / arm64).
- 🔑 Authenticates using your existing Azure CLI or Azure PowerShell login.
- 🔍 Filter and fuzzy-search hundreds of eligible scopes, then activate multiple roles in one go.
- 🧭 Narrow management group activations down to a single subscription or resource group when needed.

## 🚀 Quick Install

### 🍏 macOS / Linux

```shell
curl -sSfL https://raw.githubusercontent.com/jeircul/pim/main/scripts/install.sh | bash
```

Install a specific release (defaults to the latest):

```shell
curl -sSfL https://raw.githubusercontent.com/jeircul/pim/main/scripts/install.sh | bash -s -- v1.2.3
```

### 🪟 Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/jeircul/pim/main/scripts/install.ps1 | iex
```

Install a specific release:

```powershell
irm https://raw.githubusercontent.com/jeircul/pim/main/scripts/install.ps1 -OutFile install.ps1
./install.ps1 -Version v1.2.3
```

Make sure the install directory (`~/.local/bin` on Unix, `%LOCALAPPDATA%\Programs\pim` on Windows) is on your `PATH`.

## 🧰 Usage

```shell
# Show current version
pim --version

# Elevate a role for 4 hours with justification
pim -j "Deploy infrastructure" -t 4

# End the active assignment early
pim -d

# Check active elevations
pim -s

# Discover available options
pim -h

# Activate matching subscription roles without a prompt
pim -j "Deploy fix" --subscription platform-hub --role contributor

# Elevate multiple scopes in one pass (comma-separated selection)
pim -j "Investigate incident" --mg tenant-root-group

# Activate a management-group eligible role on a single subscription
pim -j "Scope to subscription" --mg org-platform --subscription finance-prod

# Headlessly scope to a resource group beneath a subscription
pim -j "Focus on resource group" --subscription finance-prod --resource-group analytics-rg

```

### 🔎 Filtering & search helpers

- `--management-group`, `--subscription`, `--scope-contains`, and `--role` narrow the eligible list (flags can repeat).
- `--resource-group` targets a specific resource group beneath a subscription.
- When a single role matches your filters the activation is queued immediately—perfect for scripts.
- During the prompt, type free-form text to fuzzy search, `all` to view the full list again, or `1,4,7` to activate several roles at once.
- Management group roles prompt you to choose child subscriptions or resource groups—filters auto-select them when only one match remains.

Sign in ahead of time with `az login` (bash/zsh) or `Connect-AzAccount` (PowerShell). The CLI automatically reuses whichever session is available. Set `PIM_ALLOW_DEVICE_LOGIN=true` if you want the tool to fall back to interactive device code prompts.

## 📦 Download Options

- **Install scripts:** see the commands above for macOS/Linux (`install.sh`) and Windows (`install.ps1`).
- **Manual download:** grab the latest release archives from [github.com/jeircul/pim/releases](https://github.com/jeircul/pim/releases).

Report issues or request features in the GitHub repository. Happy elevating! 🚀
