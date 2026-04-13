#!/bin/sh
# nix-setup.sh - Core Nix Bootstrap
# Purpose: Bootstrap nix/devbox environment using host's nix store (deskrun pattern)

set -e

# Color output helpers
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[nix-setup]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[nix-setup]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[nix-setup]${NC} $1"
}

log_error() {
    echo -e "${RED}[nix-setup]${NC} $1" >&2
}

fail() {
    log_error "$1"
    exit 1
}

# ============================================================================
# Phase 0: Get Static Busybox (works after /nix/store mount)
# ============================================================================
log_info "Phase 0: Preparing bootstrap environment..."

# Create bootstrap directory
BOOTSTRAP_DIR="/tmp/bootstrap/bin"
mkdir -p "$BOOTSTRAP_DIR"

# Download static busybox from Alpine (doesn't depend on /nix/store)
# This is needed because Nixery binaries depend on libraries in /nix/store,
# which becomes inaccessible after we bind-mount /nix/store-host over it
log_info "Downloading static busybox from Alpine..."
BUSYBOX_URL="https://busybox.net/downloads/binaries/1.35.0-x86_64-linux-musl/busybox"
if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "$BOOTSTRAP_DIR/busybox" "$BUSYBOX_URL" || {
        log_warn "Failed to download busybox, falling back to Nix version"
        BUSYBOX_PATH=$(ls -d /nix/store/*-busybox-*/bin/busybox 2>/dev/null | head -n 1)
        if [ -n "$BUSYBOX_PATH" ]; then
            cp -L "$BUSYBOX_PATH" "$BOOTSTRAP_DIR/busybox"
        else
            fail "No busybox available"
        fi
    }
elif command -v wget >/dev/null 2>&1; then
    wget -q -O "$BOOTSTRAP_DIR/busybox" "$BUSYBOX_URL" || {
        log_warn "Failed to download busybox, falling back to Nix version"
        BUSYBOX_PATH=$(ls -d /nix/store/*-busybox-*/bin/busybox 2>/dev/null | head -n 1)
        if [ -n "$BUSYBOX_PATH" ]; then
            cp -L "$BUSYBOX_PATH" "$BOOTSTRAP_DIR/busybox"
        else
            fail "No busybox available"
        fi
    }
else
    log_info "No curl/wget, using Nix busybox"
    BUSYBOX_PATH=$(ls -d /nix/store/*-busybox-*/bin/busybox 2>/dev/null | head -n 1)
    if [ -n "$BUSYBOX_PATH" ]; then
        cp -L "$BUSYBOX_PATH" "$BOOTSTRAP_DIR/busybox"
    else
        fail "No busybox available"
    fi
fi

chmod +x "$BOOTSTRAP_DIR/busybox"

# Create symlinks for essential commands
log_info "Creating busybox symlinks..."
for cmd in sh mount mkdir ls find cat grep head tail dirname basename wc tr cut sort uniq; do
    ln -sf busybox "$BOOTSTRAP_DIR/$cmd"
done

# Install busybox to /bin so it works after /nix/store is mounted
log_info "Installing busybox to /bin..."
cp "$BOOTSTRAP_DIR/busybox" /bin/busybox
chmod +x /bin/busybox
for cmd in sh mount mkdir ls find cat grep head tail dirname basename wc tr cut sort uniq; do
    ln -sf busybox "/bin/$cmd"
done
# Also create ash symlink (busybox sh is actually ash)
ln -sf busybox "/bin/ash"
log_success "Busybox installed to /bin"

# Copy SSL CA bundle
log_info "Copying SSL certificates..."
CA_BUNDLE=$(ls /nix/store/*/etc/ssl/certs/ca-bundle.crt 2>/dev/null | head -n 1)
if [ -n "$CA_BUNDLE" ]; then
    mkdir -p /etc/ssl/certs
    cp "$CA_BUNDLE" /etc/ssl/certs/ca-bundle.crt
    log_success "SSL certificates copied"
else
    log_warn "SSL CA bundle not found - TLS may not work"
fi

# Add bootstrap dir to PATH (but /bin will take precedence after mount)
export PATH="/bin:$BOOTSTRAP_DIR:$PATH"
log_success "Phase 0 complete - bootstrap environment ready"

# ============================================================================
# Phase 0.5: Setup GitHub Workspace Directories
# ============================================================================
log_info "Phase 0.5: Setting up GitHub workspace directories..."

# Copy GitHub workflow directory
if [ -d "/__w/_temp/_github_workflow" ]; then
    log_info "Copying /__w/_temp/_github_workflow to /github/workflow..."
    mkdir -p /github/workflow
    if ! cp -r /__w/_temp/_github_workflow/* /github/workflow/ 2>/dev/null; then
        log_warn "Failed to copy some files from /__w/_temp/_github_workflow (may be empty or permission issue)"
    fi
    log_success "GitHub workflow directory setup complete"
else
    log_info "No /__w/_temp/_github_workflow found - skipping (non-deskrun environment)"
fi

# Copy GitHub home directory
if [ -d "/__w/_temp/_github_home" ]; then
    log_info "Copying /__w/_temp/_github_home to /github/home..."
    mkdir -p /github/home
    if ! cp -r /__w/_temp/_github_home/* /github/home/ 2>/dev/null; then
        log_warn "Failed to copy some files from /__w/_temp/_github_home (may be empty or permission issue)"
    fi
    log_success "GitHub home directory setup complete"
else
    log_info "No /__w/_temp/_github_home found - skipping (non-deskrun environment)"
fi

log_success "Phase 0.5 complete - GitHub workspace setup done"

# ============================================================================
# Phase 1: Mount Host Store and Find Host Nix
# ============================================================================
log_info "Phase 1: Mounting host nix store..."

# Verify /nix/store-host exists
if [ ! -d "/nix/store-host" ]; then
    fail "/nix/store-host not found - ensure deskrun runner has mounted host's nix store"
fi

# Verify daemon socket exists
if [ ! -d "/nix/var/nix/daemon-socket-host" ]; then
    fail "/nix/var/nix/daemon-socket-host not found - ensure deskrun runner has mounted daemon socket"
fi

# Mount host store
log_info "Mounting /nix/store-host to /nix/store..."
mount --bind /nix/store-host /nix/store
log_success "Host store mounted"

# Mount daemon socket
# Note: The ARC runner mounts the socket FILE directly at /nix/var/nix/daemon-socket-host/socket
# Using bind mount on the directory would convert the socket from type 'srw-rw-rw-' to '-rw-r--r--'
# Instead, we create a symlink to preserve the socket type
log_info "Setting up daemon socket..."
mkdir -p /nix/var/nix/daemon-socket
ln -sf /nix/var/nix/daemon-socket-host/socket /nix/var/nix/daemon-socket/socket
log_success "Daemon socket symlinked"

# Find nix-env in host store (faster than find)
log_info "Finding nix in host store..."
NIX_ENV_PATH=$(ls -d /nix/store/*-nix-*/bin/nix-env 2>/dev/null | head -n 1)
if [ -z "$NIX_ENV_PATH" ]; then
    fail "nix-env not found in /nix/store - ensure host has nix installed"
fi

NIX_BIN_DIR=$(dirname "$NIX_ENV_PATH")
log_success "Found nix at: $NIX_BIN_DIR"

# Add nix bin directory to PATH
export PATH="$NIX_BIN_DIR:$PATH"
log_success "Phase 1 complete - nix available in PATH"

# ============================================================================
# Phase 2: Configure Nix Environment
# ============================================================================
log_info "Phase 2: Configuring nix environment..."

# Create nix.conf
log_info "Creating /etc/nix/nix.conf..."
mkdir -p /etc/nix
cat > /etc/nix/nix.conf <<EOF
build-users-group =
experimental-features = nix-command flakes
ssl-cert-file = /etc/ssl/certs/ca-bundle.crt
EOF
log_success "nix.conf created"

# Export environment variables
export NIX_REMOTE=daemon
export NIX_DAEMON_SOCKET_PATH=/nix/var/nix/daemon-socket/socket
export NIX_SSL_CERT_FILE=/etc/ssl/certs/ca-bundle.crt
log_success "Environment variables configured"

# Test nix is working
log_info "Testing nix connection..."
if nix-env --version >/dev/null 2>&1; then
    log_success "Nix is working: $(nix-env --version)"
else
    fail "Nix test failed - nix-env not working"
fi

log_success "Phase 2 complete - nix configured and tested"

# ============================================================================
# Export to GITHUB_ENV
# ============================================================================
if [ -n "$GITHUB_ENV" ]; then
    log_info "Exporting environment variables to GITHUB_ENV..."
    
    # Build an updated PATH that includes required entries without duplicating them
    UPDATED_PATH="$PATH"
    for _p in "$HOME/.nix-profile/bin" "/tmp/bootstrap/bin"; do
        case ":$UPDATED_PATH:" in
            *":$_p:"*) ;;
            *) UPDATED_PATH="$_p:$UPDATED_PATH" ;;
        esac
    done
    
    {
        echo "NIX_REMOTE=daemon"
        echo "NIX_DAEMON_SOCKET_PATH=/nix/var/nix/daemon-socket/socket"
        echo "NIX_SSL_CERT_FILE=/etc/ssl/certs/ca-bundle.crt"
        echo "SSL_CERT_FILE=/etc/ssl/certs/ca-bundle.crt"
        echo "CURL_CA_BUNDLE=/etc/ssl/certs/ca-bundle.crt"
        echo "PATH=$UPDATED_PATH"
    } >> "$GITHUB_ENV"
    log_success "Environment variables exported to GITHUB_ENV"
else
    log_warn "GITHUB_ENV not set - environment variables not persisted for future steps"
fi

log_success "✅ Nix setup complete!"
