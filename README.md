# instant-bosh

A containerized BOSH director for local development and testing.

## Features

- BOSH director running in a container
- **Docker CPI**: Deploy VMs as Docker containers  
- **Incus CPI**: Deploy VMs as Incus virtual machines
- Automatic cloud-config setup
- SSH support for VMs (via runtime-config)
- Jumpbox user for proxying SSH connections

## Installation

### Pre-built Binaries (Recommended)

Download the latest pre-built binary for your platform from the [releases page](https://github.com/rkoster/instant-bosh/releases/latest):

**Linux (x86_64):**
```bash
curl -sL https://github.com/rkoster/instant-bosh/releases/latest/download/instant-bosh_Linux_x86_64.tar.gz | tar xz
sudo mv ibosh /usr/local/bin/
```

**Linux (ARM64):**
```bash
curl -sL https://github.com/rkoster/instant-bosh/releases/latest/download/instant-bosh_Linux_arm64.tar.gz | tar xz
sudo mv ibosh /usr/local/bin/
```

**macOS (Intel):**
```bash
curl -sL https://github.com/rkoster/instant-bosh/releases/latest/download/instant-bosh_Darwin_x86_64.tar.gz | tar xz
sudo mv ibosh /usr/local/bin/
```

**macOS (Apple Silicon):**
```bash
curl -sL https://github.com/rkoster/instant-bosh/releases/latest/download/instant-bosh_Darwin_arm64.tar.gz | tar xz
sudo mv ibosh /usr/local/bin/
```

**Windows:**
Download the zip file from the [releases page](https://github.com/rkoster/instant-bosh/releases/latest), extract it, and add `ibosh.exe` to your PATH.

### Using Nix Flakes

You can also install instant-bosh using Nix flakes:

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

### Docker Backend (Default)

```bash
# Start the director using Docker backend
ibosh docker start

# Set BOSH CLI environment variables
eval "$(ibosh docker print-env)"

# Verify BOSH is running
bosh env

# Stop the director
ibosh docker stop

# Completely destroy instant-bosh and all resources
ibosh docker destroy
```

### Incus Backend

```bash
# Start the director using Incus backend
ibosh incus start

# Set BOSH CLI environment variables
eval "$(ibosh incus print-env)"

# Verify BOSH is running
bosh env

# Stop the director
ibosh incus stop

# Completely destroy instant-bosh and all resources
ibosh incus destroy
```

## Usage

### ibosh CLI Commands

The CLI is organized into backend-specific subcommands:

```
NAME:
   ibosh - instant-bosh CLI

USAGE:
   ibosh [global options] command [command options]

COMMANDS:
   docker   Docker backend commands
   incus    Incus backend commands
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --debug, -d    Enable debug logging (default: false)
   --help, -h     show help
   --version, -v  print the version
```

### Docker Backend Commands

```bash
ibosh docker start           # Start instant-bosh director
ibosh docker stop            # Stop instant-bosh director
ibosh docker destroy [-f]    # Destroy instant-bosh and all data
ibosh docker logs [-f]       # Show logs from the container
ibosh docker env             # Show environment info
ibosh docker print-env       # Print BOSH CLI environment variables
ibosh docker upload-stemcell <image>  # Upload a light stemcell
```

**Docker Start Options:**
- `--skip-update`: Skip checking for image updates
- `--skip-stemcell-upload`: Skip automatic stemcell upload
- `--image`: Use a custom image (e.g., `ghcr.io/rkoster/instant-bosh:main-9e61f6f`)

### Incus Backend Commands

```bash
ibosh incus start            # Start instant-bosh director
ibosh incus stop             # Stop instant-bosh director
ibosh incus destroy [-f]     # Destroy instant-bosh and all data
ibosh incus env              # Show environment info
ibosh incus print-env        # Print BOSH CLI environment variables
```

**Incus Start Options:**
- `--remote`: Incus remote name (env: `IBOSH_INCUS_REMOTE`)
- `--network`: Incus network name (env: `IBOSH_INCUS_NETWORK`)
- `--storage-pool`: Incus storage pool name (env: `IBOSH_INCUS_STORAGE_POOL`, default: `default`)
- `--project`: Incus project name (env: `IBOSH_INCUS_PROJECT`, default: `default`)
- `--image`: Use a custom image

### Deploying Workloads

After starting instant-bosh with `ibosh docker start` (or `ibosh incus start`) and setting the environment with `eval "$(ibosh docker print-env)"` (or `eval "$(ibosh incus print-env)"`), you can deploy BOSH releases:

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

- **Director**: BOSH director running in a container
- **Docker CPI**: Cloud Provider Interface for creating VMs as Docker containers
- **Incus CPI**: Cloud Provider Interface for creating VMs as Incus virtual machines
- **Jumpbox**: SSH gateway running on the director (port 2222)
- **Runtime Config**: Automatically enables SSH on all VMs

### VM SSH Support

Since systemd services don't auto-start in Docker containers, SSH must be explicitly started on VMs. This is handled automatically via:

1. **os-conf-release**: Provides the `pre-start-script` job
2. **Runtime Config** (`runtime-config-enable-vm-ssh.yml`): Applies the SSH startup script to all VMs

The `ibosh docker start` and `ibosh incus start` commands automatically:
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