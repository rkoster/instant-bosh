#!/bin/bash
# Script to start Colima with Rosetta emulation for Apple Silicon Macs
# This enables running x86_64 containers with better compatibility than QEMU
#
# Rosetta is required for instant-bosh because:
# - The instant-bosh Docker image is built for amd64 (x86_64)
# - QEMU emulation has issues with runc's /proc/self/exe cloning mechanism
# - Rosetta provides faster and more compatible x86_64 emulation
#
# Prerequisites:
# - macOS Ventura (13.0) or later
# - Apple Silicon Mac (M1/M2/M3)
# - Rosetta 2 installed (softwareupdate --install-rosetta)
#
# Usage: ./scripts/start-colima-rosetta.sh [--force]
#
# Options:
#   --force    Stop and restart Colima even if already running

set -euo pipefail

# Configuration - adjust these as needed
CPU="${COLIMA_CPU:-8}"
MEMORY="${COLIMA_MEMORY:-16}"
DISK="${COLIMA_DISK:-200}"

# Parse arguments
FORCE_RESTART=false
while [[ $# -gt 0 ]]; do
  case $1 in
    --force)
      FORCE_RESTART=true
      shift
      ;;
    *)
      echo "Unknown option: $1"
      echo "Usage: $0 [--force]"
      exit 1
      ;;
  esac
done

# Check if running on Apple Silicon
check_apple_silicon() {
  if [[ "$(uname -m)" != "arm64" ]]; then
    echo "Error: This script is intended for Apple Silicon Macs only."
    echo "Current architecture: $(uname -m)"
    exit 1
  fi
}

# Check macOS version (need Ventura or later for vz + rosetta)
check_macos_version() {
  local version
  version=$(sw_vers -productVersion)
  local major_version
  major_version=$(echo "$version" | cut -d. -f1)
  
  if [[ "$major_version" -lt 13 ]]; then
    echo "Error: macOS Ventura (13.0) or later is required for Rosetta + Virtualization.framework"
    echo "Current version: $version"
    exit 1
  fi
}

# Check if Rosetta is installed
check_rosetta() {
  if ! /usr/bin/pgrep -q oahd 2>/dev/null; then
    echo "Rosetta 2 does not appear to be installed or running."
    echo "Installing Rosetta 2..."
    softwareupdate --install-rosetta --agree-to-license
  fi
}

# Check if Colima is installed
check_colima() {
  if ! command -v colima &>/dev/null; then
    echo "Error: Colima is not installed."
    echo "Install it with: brew install colima"
    exit 1
  fi
}

# Stop Colima if running and force flag is set
stop_colima_if_needed() {
  if colima status &>/dev/null; then
    if [[ "$FORCE_RESTART" == "true" ]]; then
      echo "==> Stopping existing Colima instance..."
      colima stop
    else
      echo "Colima is already running."
      echo "Use --force to stop and restart with Rosetta configuration."
      
      # Check if current config already uses Rosetta
      if colima ssh -- cat /proc/sys/fs/binfmt_misc/rosetta &>/dev/null; then
        echo "Rosetta emulation is already enabled."
        exit 0
      else
        echo "Warning: Current Colima instance is NOT using Rosetta."
        echo "Run with --force to restart with Rosetta support."
        exit 1
      fi
    fi
  fi
}

# Start Colima with Rosetta
start_colima() {
  echo "==> Starting Colima with Rosetta emulation..."
  echo "    CPU: ${CPU}"
  echo "    Memory: ${MEMORY}GiB"
  echo "    Disk: ${DISK}GiB"
  echo ""
  
  # Key options:
  # --arch aarch64      : Run VM natively on ARM (required for vz-rosetta)
  # --vm-type vz        : Use Apple's Virtualization.framework
  # --vz-rosetta        : Enable Rosetta for x86_64 binary translation
  # --network-address   : Assign a routable IP to the VM
  # --mount-type virtiofs: Fast file sharing
  # --binfmt            : Register binfmt handlers (needed for multi-arch)
  
  colima start \
    --cpu "${CPU}" \
    --memory "${MEMORY}" \
    --disk "${DISK}" \
    --arch aarch64 \
    --vm-type vz \
    --vz-rosetta \
    --port-forwarder=ssh \
    --network-address \
    --mount-type=virtiofs \
    --mount "${TMPDIR}:w" \
    --mount "${HOME}:w"
  
  # Fix Docker socket permissions
  echo "==> Fixing Docker socket permissions..."
  colima ssh -- sudo chmod 666 /var/run/docker.sock
}

# Verify Rosetta is working
verify_rosetta() {
  echo "==> Verifying Rosetta emulation..."
  
  # Check if Rosetta binfmt is registered
  if colima ssh -- cat /proc/sys/fs/binfmt_misc/rosetta &>/dev/null; then
    echo "    Rosetta binfmt handler: REGISTERED"
  else
    echo "    Warning: Rosetta binfmt handler not found"
    echo "    Available handlers:"
    colima ssh -- ls /proc/sys/fs/binfmt_misc/
  fi
  
  # Test running an x86_64 binary
  echo "==> Testing x86_64 emulation..."
  if docker run --rm --platform linux/amd64 alpine:latest uname -m 2>/dev/null | grep -q x86_64; then
    echo "    x86_64 container test: PASSED"
  else
    echo "    Warning: x86_64 container test failed"
  fi
}

# Print summary
print_summary() {
  echo ""
  echo "=============================================="
  echo "Colima started with Rosetta emulation"
  echo "=============================================="
  echo ""
  colima list
  echo ""
  echo "Docker context is ready. You can now run:"
  echo "  go run ./cmd/ibosh/main.go docker start"
  echo ""
}

# Main
main() {
  echo "=============================================="
  echo "Starting Colima with Rosetta for Apple Silicon"
  echo "=============================================="
  echo ""
  
  check_apple_silicon
  check_macos_version
  check_rosetta
  check_colima
  stop_colima_if_needed
  start_colima
  verify_rosetta
  print_summary
}

main "$@"
