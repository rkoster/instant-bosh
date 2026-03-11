#!/bin/bash
# Build a patched instant-bosh director image for Apple Silicon
#
# This script builds a Docker image that includes a patched BPM binary
# compiled from a branch with Rosetta emulation detection. The patched BPM
# automatically disables seccomp when running under architecture emulation.
#
# Usage:
#   ./build.sh                                      # Build from latest
#   ./build.sh ghcr.io/rkoster/instant-bosh:sha-xxx # Build from specific image
#
# Environment variables:
#   OUTPUT_IMAGE - Override the output image name/tag
#   BPM_BRANCH   - Override the BPM branch to build from (default: disable-seccomp-for-docker-cpi-on-apple-silicon)
#   BPM_REPO     - Override the BPM repository URL (default: https://github.com/cloudfoundry/bpm-release.git)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Default base image - use the argument or default to latest
BASE_IMAGE="${1:-ghcr.io/rkoster/instant-bosh:latest}"

# BPM build configuration
BPM_BRANCH="${BPM_BRANCH:-disable-seccomp-for-docker-cpi-on-apple-silicon}"
BPM_REPO="${BPM_REPO:-https://github.com/cloudfoundry/bpm-release.git}"

# Output image - derive from base image or use override
if [[ -n "${OUTPUT_IMAGE:-}" ]]; then
    OUTPUT="${OUTPUT_IMAGE}"
else
    # Extract tag from base image and append -apple-silicon
    if [[ "${BASE_IMAGE}" =~ :(.+)$ ]]; then
        TAG="${BASH_REMATCH[1]}"
        OUTPUT="${BASE_IMAGE/:${TAG}/:${TAG}-apple-silicon}"
    else
        OUTPUT="${BASE_IMAGE}:apple-silicon"
    fi
fi

echo "=============================================="
echo "Building Patched instant-bosh for Apple Silicon"
echo "=============================================="
echo ""
echo "Base image:   ${BASE_IMAGE}"
echo "Output image: ${OUTPUT}"
echo "BPM branch:   ${BPM_BRANCH}"
echo "BPM repo:     ${BPM_REPO}"
echo ""

# Build the image (multi-stage: compiles BPM from source)
echo "Building image (this may take a few minutes to compile BPM)..."
docker build \
    --platform linux/amd64 \
    --provenance=false \
    --build-arg "BASE_IMAGE=${BASE_IMAGE}" \
    --build-arg "BPM_BRANCH=${BPM_BRANCH}" \
    --build-arg "BPM_REPO=${BPM_REPO}" \
    -t "${OUTPUT}" \
    -f "${SCRIPT_DIR}/Dockerfile" \
    "${SCRIPT_DIR}"

echo ""
echo "=============================================="
echo "Build Complete!"
echo "=============================================="
echo ""
echo "Tagged as: ${OUTPUT}"
echo ""
echo "To use this image with instant-bosh, set IBOSH_IMAGE:"
echo "  export IBOSH_IMAGE=${OUTPUT}"
echo "  ibosh start"
echo ""
echo "Or push to a registry and use:"
echo "  docker push ${OUTPUT}"
echo ""
