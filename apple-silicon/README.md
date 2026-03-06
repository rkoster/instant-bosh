# Apple Silicon Support

Run instant-bosh on Apple Silicon Macs using Colima with Rosetta x86_64 emulation.

**Goal:** Get these fixes upstream and remove this directory.

## Overview

Two types of containers need patching:

1. **Director container** - BPM seccomp filters fail on arm64 kernel; postgres refuses to run as root
2. **Stemcell VMs** - systemd services crash due to `MemoryDenyWriteExecute=yes` conflicting with Rosetta JIT

## Upstream Issues

| Issue | Workaround | Upstream Fix Needed |
|-------|------------|---------------------|
| systemd services crash in stemcell VMs | `stemcell/Dockerfile` adds drop-in overrides | Add `ConditionVirtualization=!container` or detect Rosetta |
| Seccomp filters fail (x86_64 on arm64) | `director/Dockerfile` enables BPM privileged mode | Native arm64 BOSH director image |
| Postgres refuses to run as root | `director/Dockerfile` patches startup scripts to drop privileges | BPM should handle privilege dropping |

## Directory Structure

```
apple-silicon/
├── director/
│   ├── Dockerfile      # Patches instant-bosh director templates
│   └── build.sh        # Build script
├── stemcell/
│   ├── Dockerfile      # Patches CloudFoundry stemcell images
│   └── build.sh        # Build script
├── start-colima-rosetta.sh  # Start Colima with Rosetta emulation
└── README.md
```

## Quick Start

### 1. Start Colima with Rosetta

```bash
./apple-silicon/start-colima-rosetta.sh
```

Or manually:
```bash
colima start --arch x86_64 --vm-type vz --vz-rosetta --cpu 4 --memory 8
```

### 2. Build Patched Images

Build the patched director image:
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

The Dockerfile patches BOSH release templates at build time:

1. **Adds `privileged: true`** to all BPM config templates - disables seccomp filters that fail on arm64
2. **Drops privileges to vcap user** in startup script templates for:
   - postgres (required - postgres explicitly refuses to run as root)
   - director, scheduler, sync-dns, metrics-server
   - nats, bosh_nats_sync
   - health_monitor
3. **Nginx stays as root** - needs root to bind ports, drops to 'nobody' via its own config

Key insight: We patch the *template* files in `/var/vcap/jobs/*/templates/` so fixes are applied when monit renders configs at runtime.

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

### Check stemcell VM systemd status
```bash
docker exec -it <stemcell-container-id> systemctl status
```

### View BPM logs
```bash
docker exec -it $(docker ps -qf name=instant-bosh) bash
tail -f /var/vcap/sys/log/*/bpm.log
```
