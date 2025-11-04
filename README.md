# PIM - Azure PIM CLI

Opinionated command-line tool for activating, deactivating, and checking Azure Privileged Identity Management (PIM) role assignments.

## Features

- Activate eligible Azure role assignments with justifications and custom duration.
- Deactivate active assignments in seconds.
- Inspect current elevations with `--status`.
- Works on macOS, Linux, and Windows (amd64 / arm64).
- Authenticates using your existing Azure login (device code flow).

## Quick Install

### macOS / Linux

```shell
curl -sSfL https://raw.githubusercontent.com/jeircul/pim/main/scripts/install.sh | bash
```

Install a specific release (defaults to the latest):

```shell
curl -sSfL https://raw.githubusercontent.com/jeircul/pim/main/scripts/install.sh | bash -s -- v1.2.3
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/jeircul/pim/main/scripts/install.ps1 | iex
```

Install a specific release:

```powershell
irm https://raw.githubusercontent.com/jeircul/pim/main/scripts/install.ps1 -OutFile install.ps1
./install.ps1 -Version v1.2.3
```

Make sure the install directory (`~/.local/bin` on Unix, `%LOCALAPPDATA%\Programs\pim` on Windows) is on your `PATH`.

## Usage

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

The first time you invoke a command that talks to Azure you will see a device code prompt. Open the provided URL, enter the code, and sign in with the account that has PIM access. Subsequent commands reuse the same sign-in until the token expires.

## Download Options

- **Install scripts:** see the commands above for macOS/Linux (`install.sh`) and Windows (`install.ps1`).
- **Manual download:** grab the latest release archives from [github.com/jeircul/pim/releases](https://github.com/jeircul/pim/releases).

Report issues or request features in the GitHub repository. Happy elevating!
