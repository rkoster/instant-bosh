# Contributing to instant-bosh

Thank you for your interest in contributing to instant-bosh! This guide will help you get started with development.

## Development Setup

### Prerequisites

You can use either Nix or devbox for development:

#### Using Nix Flakes

```bash
# Enter development shell
nix develop

# Or use direnv for automatic environment loading
echo "use flake" > .envrc
direnv allow
```

#### Using devbox

```bash
# Enter devbox shell
devbox shell

# Or use direnv for automatic environment loading (if .envrc is configured for devbox)
direnv allow
```

## Building the Project

### Building the ibosh CLI

```bash
# Using devbox
devbox run build-ibosh

# Or using make
make build-ibosh

# Or directly with go
go build -o ibosh ./cmd/ibosh
```

### Building the BOSH OCI Image

The BOSH director runs as an OCI image. To build it:

```bash
make build
```

This uses [bob (BOSH OCI Builder)](https://github.com/rkoster/bosh-oci-builder) to create the director image with all necessary configurations.

For development with a local version of bob:

```bash
make dev-bob-build
```

## Available Makefile Targets

### Build Targets

- `make build` - Build BOSH OCI image using bob
- `make build-ibosh` - Build the ibosh CLI binary
- `make dev-bob-build` - Build BOSH OCI image using go run (for bob development)

### Runtime Targets

- `make run` - Run the built BOSH image using docker run (deprecated: use `ibosh start` instead)
- `make stop` - Stop the running BOSH container (deprecated: use `ibosh stop` instead)
- `make logs` - Show logs from the running BOSH container
- `make print-env` - Print environment variables for BOSH CLI (use: `eval "$(make print-env)"`)

### Maintenance Targets

- `make sync` - Sync manifest dependencies using vendir
- `make clean` - Stop container and remove image (keeps volumes)
- `make reset` - Full reset: stop container, remove volumes and image
- `make help` - Show available targets

## Running Tests

This project uses [Ginkgo](https://onsi.github.io/ginkgo/) for testing:

```bash
# Run all tests
go run github.com/onsi/ginkgo/v2/ginkgo -r

# Run tests with verbose output
go run github.com/onsi/ginkgo/v2/ginkgo -r -v
```

## Development Workflow

### 1. Sync Dependencies

When updating BOSH deployment manifests:

```bash
make sync
```

This uses [vendir](https://carvel.dev/vendir/) to sync dependencies defined in `vendir.yml`.

### 2. Make Code Changes

Make your changes to the Go code in:
- `cmd/ibosh/` - CLI entry point
- `internal/` - Internal packages

### 3. Test Your Changes

```bash
# Run tests
go run github.com/onsi/ginkgo/v2/ginkgo -r

# Build and test the CLI
make build-ibosh
./ibosh help
```

### 4. Build and Test the Director Image

```bash
# Build the director image
make build

# Start the director
ibosh start

# Test functionality
ibosh status
eval "$(ibosh print-env)"
bosh env

# Clean up
ibosh destroy
```

## Project Structure

```
.
├── cmd/ibosh/          # CLI entry point
├── internal/           # Internal packages
│   ├── commands/       # CLI command implementations
│   ├── director/       # BOSH director client
│   ├── docker/         # Docker client wrapper
│   ├── logparser/      # Log parsing utilities
│   └── logwriter/      # Log writing utilities
├── ops/                # Ops files for BOSH director customization
├── test/               # Test fixtures and manifests
├── manifests/          # BOSH deployment manifests (synced via vendir)
├── Makefile           # Build automation
├── vendir.yml         # Dependency configuration
├── devbox.json        # Devbox configuration
└── flake.nix          # Nix flake configuration
```

## Code Style

- Follow standard Go conventions
- Run `gofmt` before committing
- Write tests for new functionality
- Update documentation for user-facing changes

## Debugging

### Debug Template Rendering

To debug BOSH template rendering without running hooks:

```bash
make debug
```

### View Container Logs

```bash
# Using ibosh
ibosh logs

# Using make
make logs

# Using docker directly
docker logs instant-bosh
```

## Release Process

1. Update version numbers (if applicable)
2. Build and test locally
3. Create a pull request
4. After merge, the CI/CD pipeline will build and publish the image

## Getting Help

- Open an issue on GitHub for bugs or feature requests
- Check existing issues for known problems
- Review the README.md for user documentation
