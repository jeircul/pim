# PIM - Azure PIM CLI

Modular, production-ready CLI tool for activating and deactivating Azure Privileged Identity Management (PIM) roles.

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

# Check status of active roles
./pim -s

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

## Next Steps

After testing and verification:

1. Run tests: `task test`
2. Build for your platform: `task build`
3. Test functionality: `./pim -h`
4. Ready to use - module paths already set to `github.com/jeircul/pim`
