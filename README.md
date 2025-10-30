# PIM - Azure PIM CLI

Modular, production-ready CLI tool for activating and deactivating Azure Privileged Identity Management (PIM) roles.

## Project Structure

```shell
pim/
├── main.go                    # CLI entry point
├── pkg/azpim/                 # Reusable PIM client library (public API)
│   ├── client.go              # PIM client implementation
│   ├── types.go               # Data structures
│   ├── errors.go              # Custom errors
│   └── client_test.go         # Unit tests
├── internal/cli/              # CLI-specific code (private)
│   ├── flags.go               # Flag parsing and validation
│   ├── prompt.go              # Interactive user prompts
│   ├── handlers.go            # Activation/deactivation handlers
│   └── flags_test.go          # CLI tests
├── go.mod                     # Go module definition
├── Taskfile.yml               # Task automation
└── README.md                  # This file
```

## Features

- **Modular Design** - Reusable `pkg/azpim` library for other Go projects
- **Testable** - Unit tests for all components
- **Production-Ready** - Error handling, validation, proper structure
- **Same Functionality** - Identical to original `pim` tool
- **Task Automation** - Modern Taskfile for builds and tests

## Build

```shell
# Build for current platform
task build

# Build for specific platform
task build:linux
task build:macos
task build:windows

# Build for all platforms
task build:all
```

## Test

```shell
# Run all tests
task test

# Run unit tests only
task test:unit

# Generate coverage report
task test:coverage
```

## Usage

```shell
# Activate a role
./pim -j "Deploy infrastructure" -t 4

# Deactivate a role
./pim -d

# Show help
./pim -h
```

## Reusing the Library

The `pkg/azpim` package can be imported in other Go projects:

```go
import "github.com/jeircul/pim/pkg/azpim"

func main() {
    ctx := context.Background()
    client, _ := azpim.NewClient(ctx)

    roles, _ := client.GetEligibleRoles()
    // Use in your automation...
}
```

## Development

```shell
# Format code
task fmt

# Tidy dependencies
task tidy

# Clean build artifacts
task clean
```

## Differences from Original

1. **Modular Structure** - Separated into packages
2. **Testable** - Unit tests included
3. **Reusable** - `pkg/azpim` can be imported
4. **Better Organization** - Clear separation of concerns

## Next Steps

After testing and verification:

1. Run tests: `task test`
2. Build for your platform: `task build`
3. Test functionality: `./pim -h`
4. Ready to use - module paths already set to `github.com/jeircul/pim`
