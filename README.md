# instant-bosh

A containerized BOSH director for local development and testing.

## Features

- BOSH director running in a Docker container
- Docker CPI for deploying VMs as containers
- Automatic cloud-config setup
- SSH support for VMs (via runtime-config)
- Jumpbox user for proxying SSH connections

## Quick Start

```bash
# Build the BOSH director image
make build

# Start the director using ibosh CLI
ibosh start

# Set BOSH CLI environment variables
eval "$(make print-env)"

# Verify BOSH is running
bosh env

# Check instant-bosh status
ibosh status

# Stop the director
ibosh stop

# Completely destroy instant-bosh and all resources
ibosh destroy
```

## Usage

### Variable Interpolation

instant-bosh supports variable interpolation in cloud-config and runtime-config using BOSH-style placeholders. This allows you to customize configurations without modifying the code.

**Example vars file** (`custom-vars.yml`):
```yaml
network_name: my-network
worker_count: 10
```

**Usage:**
```bash
ibosh start --vars-file custom-vars.yml
```

Variables are interpolated using the `((variable_name))` syntax in the embedded cloud-config and runtime-config. You can specify multiple vars files, and variables from later files will override earlier ones:

```bash
ibosh start -l base-vars.yml -l override-vars.yml
```

### ibosh CLI Commands

The `ibosh` CLI provides a streamlined interface for managing instant-bosh:

- `ibosh start` - Start the instant-bosh director (creates volumes, network, and container; auto-pulls image if not available)
  - `--vars-file` or `-l` - Load variables from a YAML file for interpolation in configs (can be specified multiple times)
- `ibosh stop` - Stop the running director
- `ibosh status` - Show status of instant-bosh and containers on the network
- `ibosh destroy` - Remove all instant-bosh resources (container, volumes, network, and network containers)
- `ibosh pull` - Pull the latest instant-bosh image from the registry
- `ibosh logs` - Show logs from the instant-bosh container (use `-f` to follow, `-n` to specify number of lines)

### Available Makefile Targets

- `make build` - Build BOSH OCI image using bob
- `make logs` - Show director logs
- `make print-env` - Print BOSH CLI environment variables

### Deploying Workloads

After running `make run` and setting the environment with `eval "$(make print-env)"`, you can deploy BOSH releases:

```bash
# Deploy a sample zookeeper cluster
bosh deploy -d zookeeper test/manifest/zookeeper.yml

# SSH into a VM
bosh -d zookeeper ssh zookeeper/0

# List instances
bosh -d zookeeper instances
```

## Architecture

### Components

- **Director**: BOSH director running in a Docker container
- **Docker CPI**: Cloud Provider Interface for creating VMs as Docker containers
- **Jumpbox**: SSH gateway running on the director (port 2222)
- **Runtime Config**: Automatically enables SSH on all VMs

### VM SSH Support

Since systemd services don't auto-start in Docker containers, SSH must be explicitly started on VMs. This is handled automatically via:

1. **os-conf-release**: Provides the `pre-start-script` job
2. **Runtime Config** (`runtime-config-enable-vm-ssh.yml`): Applies the SSH startup script to all VMs

The `make run` target automatically:
- Uploads the `os-conf-release`
- Applies the runtime config
- Configures cloud-config

### BOSH CLI Proxy Setup

The `BOSH_ALL_PROXY` environment variable enables the BOSH CLI to proxy both API calls and SSH connections through the director:

```bash
BOSH_ALL_PROXY=ssh+socks5://jumpbox@localhost:2222?private-key=/tmp/jumpbox-key.XXXXXX
```

This allows `bosh ssh` to work seamlessly with VMs running as Docker containers.

## Development

### Running Tests

This project uses [Ginkgo](https://onsi.github.io/ginkgo/) for testing:

```bash
# Run all tests
go run github.com/onsi/ginkgo/v2/ginkgo -r

# Run tests with verbose output
go run github.com/onsi/ginkgo/v2/ginkgo -r -v
```

## Files

- `Makefile` - Build and run automation
- `runtime-config-enable-vm-ssh.yml` - Runtime config to enable SSH on VMs
- `ops-*.yml` - Ops files for customizing the BOSH director
- `vendor/bosh-deployment/` - Vendored BOSH deployment manifests
- `test/manifest/` - Example deployment manifests