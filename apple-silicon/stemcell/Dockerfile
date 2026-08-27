# Dockerfile for patching upstream BOSH stemcell images for Apple Silicon
#
# This patches the CloudFoundry stemcell Docker images to work under Rosetta
# x86_64 emulation on Apple Silicon Macs.
#
# The main issue is that systemd services with MemoryDenyWriteExecute=yes
# fail under Rosetta because Rosetta uses JIT compilation which requires
# writable+executable memory.
#
# Usage:
#   docker build --build-arg STEMCELL_IMAGE=ghcr.io/cloudfoundry/ubuntu-jammy-stemcell:1.586 \
#                -t my-patched-stemcell:1.586 .
#
# Or use the build.sh script:
#   ./build.sh ubuntu-jammy 1.586

ARG STEMCELL_IMAGE=ghcr.io/cloudfoundry/ubuntu-noble-stemcell:latest
FROM ${STEMCELL_IMAGE}

LABEL maintainer="instant-bosh"
LABEL description="BOSH stemcell patched for Apple Silicon (Rosetta x86_64 emulation)"

# Create systemd drop-in overrides to disable security features that conflict
# with Rosetta's JIT compilation. Rosetta needs writable+executable memory
# for its binary translation, which conflicts with MemoryDenyWriteExecute=yes.
#
# Services that need patching:
# - systemd-journald: Journal/logging service
# - systemd-resolved: DNS resolver
# - systemd-networkd: Network configuration (may be used)
# - systemd-logind: Login manager
# - systemd-timesyncd: Time synchronization
# - auditd: Audit daemon (required by bosh-agent bootstrap)

# Create a common drop-in that can be applied to multiple services
RUN mkdir -p /etc/systemd/system/systemd-journald.service.d \
             /etc/systemd/system/systemd-resolved.service.d \
             /etc/systemd/system/systemd-networkd.service.d \
             /etc/systemd/system/systemd-logind.service.d \
             /etc/systemd/system/systemd-timesyncd.service.d \
             /etc/systemd/system/auditd.service.d

# Rosetta compatibility: disable security features that conflict with JIT
# Write the config content to a temp file, then copy to each service directory
RUN printf '[Service]\n\
# Disable security features that conflict with Rosetta x86_64 emulation\n\
# Rosetta uses JIT compilation which requires writable+executable memory\n\
MemoryDenyWriteExecute=no\n\
SystemCallArchitectures=\n\
SystemCallFilter=\n\
LockPersonality=no\n\
NoNewPrivileges=no\n' > /tmp/rosetta.conf && \
    cp /tmp/rosetta.conf /etc/systemd/system/systemd-journald.service.d/rosetta.conf && \
    cp /tmp/rosetta.conf /etc/systemd/system/systemd-resolved.service.d/rosetta.conf && \
    cp /tmp/rosetta.conf /etc/systemd/system/systemd-networkd.service.d/rosetta.conf && \
    cp /tmp/rosetta.conf /etc/systemd/system/systemd-logind.service.d/rosetta.conf && \
    cp /tmp/rosetta.conf /etc/systemd/system/systemd-timesyncd.service.d/rosetta.conf && \
    cp /tmp/rosetta.conf /etc/systemd/system/auditd.service.d/rosetta.conf && \
    rm /tmp/rosetta.conf

# Also mask systemd-binfmt which fails under Rosetta (fixed upstream in PR #473)
RUN systemctl mask systemd-binfmt.service || true

# Verify the drop-ins were created
RUN cat /etc/systemd/system/systemd-journald.service.d/rosetta.conf
RUN cat /etc/systemd/system/systemd-resolved.service.d/rosetta.conf
