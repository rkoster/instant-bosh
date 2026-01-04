#!/bin/bash
# install-devbox.sh - Devbox Installation
# Purpose: Install devbox via nix profile

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[install-devbox]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[install-devbox]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[install-devbox]${NC} $1"
}

log_error() {
    echo -e "${RED}[install-devbox]${NC} $1" >&2
}

fail() {
    log_error "$1"
    exit 1
}

log_info "Starting devbox installation..."

if [ -n "$NIX_REMOTE" ]; then
    log_success "NIX_REMOTE is set to: $NIX_REMOTE"
elif command -v nix >/dev/null 2>&1; then
    log_success "nix command is available"
else
    fail "nix is not configured - please run nix-setup.sh first"
fi

log_info "Installing devbox via nix profile..."
if nix profile install nixpkgs#devbox; then
    log_success "devbox installed successfully"
else
    fail "Failed to install devbox"
fi

export PATH="$HOME/.nix-profile/bin:$PATH"
log_info "Added ~/.nix-profile/bin to PATH"

log_info "Verifying devbox installation..."
if command -v devbox >/dev/null 2>&1; then
    DEVBOX_VERSION=$(devbox version)
    log_success "devbox is available: $DEVBOX_VERSION"
else
    fail "devbox installation verification failed - command not found"
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

log_success "✅ Devbox installation complete!"
