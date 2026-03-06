#!/bin/bash
# Build a patched instant-bosh director image for Apple Silicon
#
# This script builds a Docker image from the upstream instant-bosh image
# with patches applied to work around Rosetta emulation issues.
#
# Usage:
#   ./build.sh                                      # Build from latest
#   ./build.sh ghcr.io/rkoster/instant-bosh:sha-xxx # Build from specific image
#
# Environment variables:
#   OUTPUT_IMAGE - Override the output image name/tag

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Default base image - use the argument or default to latest
BASE_IMAGE="${1:-ghcr.io/rkoster/instant-bosh:latest}"

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
echo ""

# Build the image
echo "Building image..."
docker build \
    --platform linux/amd64 \
    --provenance=false \
    --build-arg "BASE_IMAGE=${BASE_IMAGE}" \
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
