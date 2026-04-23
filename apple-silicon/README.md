# Apple Silicon Support

Run instant-bosh on Apple Silicon Macs using Colima with Rosetta x86_64 emulation.

**Goal:** Get these fixes upstream and remove this directory.

## Requirements

**Colima with VZ + Rosetta is REQUIRED.** Podman Machine is NOT supported.

### Why Podman Machine doesn't work

The BPM fix detects Rosetta emulation by checking for `/proc/sys/fs/binfmt_misc/rosetta`. Podman Machine uses **QEMU user-mode emulation** (`qemu-x86_64-static`) instead of Rosetta, even when `"Rosetta": true` is set in the machine config.

The `Rosetta: true` setting in Podman only affects the VM hypervisor layer, not container emulation inside the VM. This means:
- No `/proc/sys/fs/binfmt_misc/rosetta` exists (only `qemu-x86_64`)
- BPM cannot detect emulation and fails with seccomp errors

See [containers/podman#28181](https://github.com/containers/podman/issues/28181) for details.

### Supported Configuration

```bash
# Start Colima with Rosetta (REQUIRED)
colima start --arch aarch64 --vm-type vz --vz-rosetta --cpu 4 --memory 8
```

## Overview

Two types of containers need patching:

1. **Director container** - BPM seccomp filters fail on arm64 kernel
2. **Stemcell VMs** - systemd services crash due to `MemoryDenyWriteExecute=yes` conflicting with Rosetta JIT

## Upstream Issues

| Issue | Workaround | Upstream PR |
|-------|------------|-------------|
| Seccomp filters fail (x86_64 on arm64) | `director/Dockerfile` builds BPM with Rosetta detection | [cloudfoundry/bpm-release#201](https://github.com/cloudfoundry/bpm-release/pull/201) |
| systemd services crash in stemcell VMs | `stemcell/Dockerfile` adds drop-in overrides | Add `ConditionVirtualization=!container` or detect Rosetta |

## Directory Structure

```
apple-silicon/
├── director/
│   ├── Dockerfile                # Multi-stage build: compiles BPM from PR branch
│   ├── build.sh                  # Build script
│   └── wait-for-postgres-role.sh # Helper script for postgres startup
├── stemcell/
│   ├── Dockerfile                # Patches CloudFoundry stemcell images
│   └── build.sh                  # Build script
├── start-colima-rosetta.sh       # Start Colima with Rosetta emulation
└── README.md
```

## Quick Start

### 1. Start Colima with Rosetta

```bash
./apple-silicon/start-colima-rosetta.sh
```

Or manually:
```bash
colima start --arch aarch64 --vm-type vz --vz-rosetta --cpu 4 --memory 8
```

### 2. Build Patched Images

Build the patched director image (compiles BPM from source):
```bash
./apple-silicon/director/build.sh ghcr.io/rkoster/instant-bosh:sha-6de5b3c
```

Build the patched stemcell image:
```bash
./apple-silicon/stemcell/build.sh ubuntu-noble latest
```

### 3. Start instant-bosh with Patched Director

```bash
export IBOSH_IMAGE=ghcr.io/rkoster/instant-bosh:sha-6de5b3c-apple-silicon
ibosh start
```

### 4. Upload Patched Stemcell and Deploy

```bash
# Upload the patched stemcell
ibosh upload-stemcell ghcr.io/rkoster/ubuntu-noble-stemcell:latest-apple-silicon

# Deploy (example with zookeeper)
eval "$(ibosh print-env)"
bosh -d zookeeper deploy test/manifest/zookeeper.yml
```

## How the Patches Work

### Director Patches (`director/Dockerfile`)

The Dockerfile uses a **multi-stage build** to compile BPM from a branch that includes Rosetta emulation detection:

1. **Stage 1 (bpm-builder)**: Clones [bpm-release PR #201](https://github.com/cloudfoundry/bpm-release/pull/201) and compiles the `bpm` binary
2. **Stage 2 (final)**: Copies the patched BPM binary into the instant-bosh image

Note: The existing `runc` binary from the base image is kept as-is because the Rosetta detection logic is in the `bpm` binary's `sysfeat` package, not in runc.

**How the BPM fix works:**
- BPM now includes a `sysfeat` package that detects Rosetta emulation by checking:
  - `/proc/sys/fs/binfmt_misc/rosetta` (Rosetta binfmt registration)
  - `/proc/cpuinfo` for `VirtualApple` vendor string
- When emulation is detected, BPM sets `SeccompSupported = false`
- This causes BPM to skip loading seccomp filters (which would fail with "invalid argument")
- Unlike the previous privileged mode workaround, this **only** disables seccomp without granting additional privileges

**Environment variables for build.sh:**
- `BPM_BRANCH` - Override the BPM branch (default: `disable-seccomp-for-docker-cpi-on-apple-silicon`)
- `BPM_REPO` - Override the BPM repository URL
- `OUTPUT_IMAGE` - Override the output image name/tag

### Stemcell Patches (`stemcell/Dockerfile`)

The Dockerfile creates systemd drop-in overrides that disable security features incompatible with Rosetta JIT:

```ini
[Service]
MemoryDenyWriteExecute=no
SystemCallArchitectures=
SystemCallFilter=
LockPersonality=no
NoNewPrivileges=no
```

Services patched:
- `systemd-journald`
- `systemd-resolved`
- `systemd-networkd`
- `systemd-logind`
- `systemd-timesyncd`
- `auditd`

Also masks `systemd-binfmt.service` to prevent clearing host binfmt_misc registrations.

## Pre-built Images

If available, you can use pre-built images instead of building locally:

```bash
# Director
export IBOSH_IMAGE=ghcr.io/rkoster/instant-bosh:sha-6de5b3c-apple-silicon

# Stemcell
ibosh upload-stemcell ghcr.io/rkoster/ubuntu-noble-stemcell:latest-apple-silicon
```

## Troubleshooting

### Check if director processes are running
```bash
docker exec -it $(docker ps -qf name=instant-bosh) bash
monit summary
```

### Verify BPM version (should show `-apple-silicon` suffix)
```bash
docker exec -it $(docker ps -qf name=instant-bosh) /var/vcap/packages/bpm/bin/bpm version
```

### Check stemcell VM systemd status
```bash
docker exec -it <stemcell-container-id> systemctl status
```

### View BPM logs
```bash
docker exec -it $(docker ps -qf name=instant-bosh) bash
tail -f /var/vcap/sys/log/*/bpm.log
```

### Build with a different BPM branch
```bash
BPM_BRANCH=my-feature-branch ./apple-silicon/director/build.sh
```
