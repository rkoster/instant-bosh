#!/bin/bash
# install-docker.sh - Docker CLI Installation
# Purpose: Install docker CLI via nix profile for use with host Docker daemon

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[install-docker]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[install-docker]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[install-docker]${NC} $1"
}

log_error() {
    echo -e "${RED}[install-docker]${NC} $1" >&2
}

fail() {
    log_error "$1"
    exit 1
}

log_info "Starting docker CLI installation..."

if [ -n "$NIX_REMOTE" ]; then
    log_success "NIX_REMOTE is set to: $NIX_REMOTE"
elif command -v nix >/dev/null 2>&1; then
    log_success "nix command is available"
else
    fail "nix is not configured - please run nix-setup.sh first"
fi

log_info "Installing docker via nix profile..."
if nix profile install nixpkgs#docker; then
    log_success "docker installed successfully"
else
    fail "Failed to install docker"
fi

export PATH="$HOME/.nix-profile/bin:$PATH"
log_info "Added ~/.nix-profile/bin to PATH"

log_info "Verifying docker installation..."
if command -v docker >/dev/null 2>&1; then
    DOCKER_VERSION=$(docker --version)
    log_success "docker is available: $DOCKER_VERSION"
else
    fail "docker installation verification failed - command not found"
fi

if [ -n "$GITHUB_ENV" ]; then
    log_info "Exporting PATH to GITHUB_ENV..."
    
    UPDATED_PATH="$PATH"
    case ":$UPDATED_PATH:" in
        *":$HOME/.nix-profile/bin:"*) ;;
        *) UPDATED_PATH="$HOME/.nix-profile/bin:$UPDATED_PATH" ;;
    esac
    
    echo "PATH=$UPDATED_PATH" >> "$GITHUB_ENV"
    log_success "PATH exported to GITHUB_ENV"
else
    log_warn "GITHUB_ENV not set - PATH not persisted for future steps"
fi

log_success "✅ Docker CLI installation complete!"
log_info "Note: This only installs the docker CLI client."
log_info "For host Docker mode, ensure /var/run/docker.sock is mounted."
log_info "For Docker-in-Docker, use dind-setup.sh instead."
