# ⚡️ PIM - Azure PIM CLI

Opinionated command-line tool for activating, deactivating, and checking Azure Privileged Identity Management (PIM) role assignments.

## ✨ Features

- 🔐 Activate eligible Azure role assignments with justifications and custom duration.
- 🔄 Deactivate active assignments in seconds.
- 👀 Inspect current elevations with `--status`.
- 💻 Works on macOS, Linux, and Windows (amd64 / arm64).
- 🔑 Authenticates using your existing Azure CLI or Azure PowerShell login.

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
```

Sign in ahead of time with `az login` (bash/zsh) or `Connect-AzAccount` (PowerShell). The CLI automatically reuses whichever session is available. Set `PIM_ALLOW_DEVICE_LOGIN=true` if you want the tool to fall back to interactive device code prompts.

## 📦 Download Options

- **Install scripts:** see the commands above for macOS/Linux (`install.sh`) and Windows (`install.ps1`).
- **Manual download:** grab the latest release archives from [github.com/jeircul/pim/releases](https://github.com/jeircul/pim/releases).

Report issues or request features in the GitHub repository. Happy elevating! 🚀
