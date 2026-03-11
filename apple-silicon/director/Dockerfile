# Dockerfile for patching instant-bosh director for Apple Silicon
#
# This uses a multi-stage build to compile BPM from a branch that includes
# Rosetta emulation detection. When BPM detects it's running under Rosetta
# (x86_64 on ARM64 kernel), it automatically disables seccomp filters which
# would otherwise fail with "error loading seccomp filter: invalid argument".
#
# This approach is cleaner than the previous privileged mode workaround because
# it only disables seccomp (the actual problem) without granting additional
# privileges or requiring privilege dropping scripts.
#
# Usage:
#   docker build --platform linux/amd64 \
#                --build-arg BASE_IMAGE=ghcr.io/rkoster/instant-bosh:sha-xxx \
#                -t my-patched-instant-bosh .
#
# Or use the build.sh script:
#   ./build.sh ghcr.io/rkoster/instant-bosh:sha-xxx

# Global ARGs - must be declared before first FROM to be used in FROM lines
ARG BASE_IMAGE=ghcr.io/rkoster/instant-bosh:latest
ARG BPM_BRANCH=disable-seccomp-for-docker-cpi-on-apple-silicon
ARG BPM_REPO=https://github.com/cloudfoundry/bpm-release.git

# ============================================================================
# Stage 1: Build BPM binary from PR branch with Rosetta detection
# ============================================================================
FROM golang:1.25-bookworm AS bpm-builder

# Re-declare ARGs needed in this stage (they don't persist across FROM)
ARG BPM_BRANCH
ARG BPM_REPO

# Install git and clone the bpm-release repo
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build
RUN git clone --depth 1 --branch ${BPM_BRANCH} ${BPM_REPO} bpm-release

# Build the bpm binary
# Note: The Rosetta detection logic is in src/bpm/sysfeat/sysfeat.go
WORKDIR /build/bpm-release/src/bpm
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -mod=vendor \
    -ldflags "-X bpm/commands.Version=$(cat /build/bpm-release/src/version)-apple-silicon" \
    -o /build/bpm \
    ./cmd/bpm

# ============================================================================
# Stage 2: Final image with patched BPM binary
# ============================================================================
# Note: We only replace the bpm binary. The existing runc binary from the base
# image is kept as-is because the Rosetta detection is in BPM, not runc.
# BPM decides whether to apply seccomp filters before invoking runc.
FROM ${BASE_IMAGE}

LABEL maintainer="instant-bosh"
LABEL description="instant-bosh patched for Apple Silicon (Rosetta x86_64 emulation)"

# Copy the patched BPM binary that includes Rosetta emulation detection
# This binary automatically disables seccomp when it detects architecture mismatch
COPY --from=bpm-builder /build/bpm /var/vcap/packages/bpm/bin/bpm

# ============================================================================
# Postgres startup fix: wait for postgres role before migrations
# ============================================================================
# The create-database script runs in the background and creates the postgres role.
# We need to wait for it before running migrations.
COPY wait-for-postgres-role.sh /usr/local/bin/wait-for-postgres-role.sh
RUN chmod +x /usr/local/bin/wait-for-postgres-role.sh && \
    sed -i 's|/var/vcap/packages/director/bin/bosh-director-migrate|/usr/local/bin/wait-for-postgres-role.sh\n\n/var/vcap/packages/director/bin/bosh-director-migrate|' \
        /var/vcap/jobs/director/templates/director

# Verify the BPM binary was installed correctly
RUN /var/vcap/packages/bpm/bin/bpm version
