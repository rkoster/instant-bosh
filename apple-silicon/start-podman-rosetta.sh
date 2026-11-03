#!/bin/bash
# Script to start Podman with Rosetta emulation for Apple Silicon Macs
# This enables running x86_64 containers with better compatibility than QEMU
#
# Rosetta is required for instant-bosh because:
# - The instant-bosh Docker image is built for amd64 (x86_64)
# - QEMU emulation has issues with runc's /proc/self/exe cloning mechanism
# - Rosetta provides faster and more compatible x86_64 emulation
#
# Prerequisites:
# - macOS Tahoe (26.0) or later (Rosetta fix requires Tahoe beta or newer)
# - Apple Silicon Mac (M1/M2/M3/M4)
# - Rosetta 2 installed (softwareupdate --install-rosetta)
# - Podman 5.6 or later
#
# Based on: https://blog.podman.io/2025/08/podman-5-6-released-rosetta-status-update/
#
# Usage: ./apple-silicon/start-podman-rosetta.sh [--force]
#
# Options:
#   --force    Stop and restart Podman machine even if already running

set -euo pipefail

# Configuration - adjust these as needed
CPU="${PODMAN_CPU:-8}"
MEMORY="${PODMAN_MEMORY:-16384}"  # In MB
DISK="${PODMAN_DISK:-200}"        # In GB

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

# Check macOS version (need Tahoe or later for Rosetta fix)
check_macos_version() {
  local version
  version=$(sw_vers -productVersion)
  local major_version
  major_version=$(echo "$version" | cut -d. -f1)
  
  if [[ "$major_version" -lt 26 ]]; then
    echo "Warning: macOS Tahoe (26.0) or later is recommended for Rosetta compatibility."
    echo "Current version: $version"
    echo "Rosetta may not work correctly with Linux kernel 6.13+ on older macOS versions."
    echo ""
    read -p "Continue anyway? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
      exit 1
    fi
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

# Check if Podman is installed
check_podman() {
  if ! command -v podman &>/dev/null; then
    echo "Error: Podman is not installed."
    echo "Install it with: brew install podman"
    exit 1
  fi
  
  # Check Podman version (need 5.6+)
  local version
  version=$(podman --version | grep -oE '[0-9]+\.[0-9]+' | head -1)
  local major minor
  major=$(echo "$version" | cut -d. -f1)
  minor=$(echo "$version" | cut -d. -f2)
  
  if [[ "$major" -lt 5 ]] || { [[ "$major" -eq 5 ]] && [[ "$minor" -lt 6 ]]; }; then
    echo "Error: Podman 5.6 or later is required for Rosetta support."
    echo "Current version: $(podman --version)"
    echo "Update with: brew upgrade podman"
    exit 1
  fi
  
  echo "Podman version: $(podman --version)"
}

# Check if Podman machine exists
machine_exists() {
  podman machine list --format "{{.Name}}" 2>/dev/null | grep -q "^podman-machine-default$"
}

# Check if Podman machine is running
machine_running() {
  podman machine list --format "{{.Name}} {{.Running}}" 2>/dev/null | grep -q "^podman-machine-default true$"
}

# Stop Podman machine if running and force flag is set
stop_machine_if_needed() {
  if machine_running; then
    if [[ "$FORCE_RESTART" == "true" ]]; then
      echo "==> Stopping existing Podman machine..."
      podman machine stop
    else
      echo "Podman machine is already running."
      echo "Use --force to stop and restart with Rosetta configuration."
      
      # Check if Rosetta is already enabled
      if podman machine ssh "cat /proc/sys/fs/binfmt_misc/rosetta" &>/dev/null; then
        echo "Rosetta emulation is already enabled."
        exit 0
      else
        echo "Warning: Current Podman machine is NOT using Rosetta."
        echo "Run with --force to restart with Rosetta support."
        exit 1
      fi
    fi
  fi
}

# Initialize Podman machine if it doesn't exist
init_machine() {
  if ! machine_exists; then
    echo "==> Initializing Podman machine..."
    echo "    CPU: ${CPU}"
    echo "    Memory: ${MEMORY}MB"
    echo "    Disk: ${DISK}GB"
    echo ""
    
    podman machine init \
      --cpus "${CPU}" \
      --memory "${MEMORY}" \
      --disk-size "${DISK}"
  fi
}

# Start Podman machine
start_machine() {
  echo "==> Starting Podman machine..."
  podman machine start
}

# Enable Rosetta in the Podman machine
# Based on: https://blog.podman.io/2025/08/podman-5-6-released-rosetta-status-update/
enable_rosetta() {
  echo "==> Enabling Rosetta emulation..."
  
  # Create the Rosetta enablement file
  podman machine ssh "sudo touch /etc/containers/enable-rosetta"
  
  echo "==> Restarting Podman machine to apply Rosetta configuration..."
  podman machine stop
  podman machine start
}

# Verify Rosetta is working
verify_rosetta() {
  echo "==> Verifying Rosetta emulation..."
  
  # Check if Rosetta binfmt is registered
  if podman machine ssh "cat /proc/sys/fs/binfmt_misc/rosetta" &>/dev/null; then
    echo "    Rosetta binfmt handler: REGISTERED"
  else
    echo "    Warning: Rosetta binfmt handler not found"
    echo "    Available handlers:"
    podman machine ssh "ls /proc/sys/fs/binfmt_misc/"
    return 1
  fi
  
  # Test running an x86_64 binary
  echo "==> Testing x86_64 emulation..."
  if podman run --rm --platform linux/amd64 alpine:latest uname -m 2>/dev/null | grep -q x86_64; then
    echo "    x86_64 container test: PASSED"
  else
    echo "    Warning: x86_64 container test failed"
  fi
}

# Print summary
print_summary() {
  echo ""
  echo "=============================================="
  echo "Podman started with Rosetta emulation"
  echo "=============================================="
  echo ""
  podman machine list
  echo ""
  echo "Podman is ready. You can now run:"
  echo "  go run ./cmd/ibosh/main.go docker start"
  echo ""
  echo "Or use podman directly:"
  echo "  podman run --rm --platform linux/amd64 ghcr.io/rkoster/instant-bosh"
  echo ""
}

# Main
main() {
  echo "=============================================="
  echo "Starting Podman with Rosetta for Apple Silicon"
  echo "=============================================="
  echo ""
  
  check_apple_silicon
  check_macos_version
  check_rosetta
  check_podman
  stop_machine_if_needed
  init_machine
  start_machine
  enable_rosetta
  verify_rosetta
  print_summary
}

main "$@"
