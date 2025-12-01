# instant-bosh

A containerized BOSH director for local development and testing.

## Features

- BOSH director running in a Docker container
- Docker CPI for deploying VMs as containers
- Automatic cloud-config setup
- SSH support for VMs (via runtime-config)
- Jumpbox user for proxying SSH connections

## Installation

### Using Nix Flakes

The easiest way to install instant-bosh is using Nix flakes:

```bash
# Install directly from GitHub
nix profile install github:rkoster/instant-bosh

# Or run without installing
nix run github:rkoster/instant-bosh -- help

# Or add to your flake.nix inputs
inputs.instant-bosh.url = "github:rkoster/instant-bosh";
```

### Building from Source

If you want to build from source, see [CONTRIBUTING.md](CONTRIBUTING.md) for development setup instructions.

## Quick Start

```bash
# Start the director using ibosh CLI
ibosh start

# Set BOSH CLI environment variables
eval "$(ibosh print-env)"

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

### ibosh CLI Commands

```
NAME:
   ibosh - instant-bosh CLI

USAGE:
   ibosh [global options] command [command options]

COMMANDS:
   start      Start instant-bosh director
   stop       Stop instant-bosh director
   destroy    Destroy instant-bosh director and all data
   status     Show status of instant-bosh and containers on the network
   pull       Pull latest instant-bosh image
   print-env  Print environment variables for BOSH CLI
   logs       Show logs from the instant-bosh container
   help, h    Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --debug, -d  Enable debug logging (default: false)
   --help, -h   show help
```

### Deploying Workloads

After starting instant-bosh with `ibosh start` and setting the environment with `eval "$(ibosh print-env)"`, you can deploy BOSH releases:

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

The `ibosh start` command automatically:
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

For information on contributing to instant-bosh, building from source, running tests, and understanding the project structure, please see [CONTRIBUTING.md](CONTRIBUTING.md).

## License

This project is licensed under the Business Source License 1.1 - see the [LICENSE](LICENSE) file for details.