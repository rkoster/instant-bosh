#!/bin/bash
# Build a patched stemcell image for Apple Silicon (Rosetta x86_64 emulation)
#
# This script builds a Docker image from the upstream CloudFoundry stemcell
# with patches applied to work around Rosetta emulation issues.
#
# Usage:
#   ./build.sh                                    # Build ubuntu-noble:latest
#   ./build.sh ubuntu-noble                       # Build ubuntu-noble:latest  
#   ./build.sh ubuntu-noble 1.586                 # Build ubuntu-noble:1.586
#   ./build.sh ubuntu-jammy 1.586                 # Build ubuntu-jammy:1.586
#
# Environment variables:
#   REGISTRY - Registry to push to (default: ghcr.io/rkoster)
#              Set to your own registry namespace
#
# The BOSH Docker CPI needs to pull the image, so it must be pushed to a
# registry accessible from inside the instant-bosh container.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Default values
OS="${1:-ubuntu-noble}"
VERSION="${2:-latest}"

# Registry - default to rkoster's namespace, override with REGISTRY env var
REGISTRY="${REGISTRY:-ghcr.io/rkoster}"

# Upstream image reference
UPSTREAM_IMAGE="ghcr.io/cloudfoundry/${OS}-stemcell:${VERSION}"

# Output image - use custom registry with apple-silicon suffix
OUTPUT_TAG="${VERSION}-apple-silicon"
OUTPUT_IMAGE="${REGISTRY}/${OS}-stemcell:${OUTPUT_TAG}"

echo "=============================================="
echo "Building Patched Stemcell for Apple Silicon"
echo "=============================================="
echo ""
echo "Upstream image: ${UPSTREAM_IMAGE}"
echo "Output image:   ${OUTPUT_IMAGE}"
echo ""

# Build the image
# Use --provenance=false to avoid creating a multi-arch manifest with attestations
# This ensures the image can be pulled without specifying --platform
echo "Building image..."
docker build \
    --platform linux/amd64 \
    --provenance=false \
    --build-arg "STEMCELL_IMAGE=${UPSTREAM_IMAGE}" \
    -t "${OUTPUT_IMAGE}" \
    -f "${SCRIPT_DIR}/Dockerfile" \
    "${SCRIPT_DIR}"

echo ""
echo "=============================================="
echo "Build Complete!"
echo "=============================================="
echo ""
echo "Tagged as: ${OUTPUT_IMAGE}"
echo ""
echo "Next steps:"
echo ""
echo "  1. Push to registry (required for BOSH CPI to access):"
echo "     docker push ${OUTPUT_IMAGE}"
echo ""
echo "  2. Upload to BOSH:"
echo "     ibosh upload-stemcell ${OUTPUT_IMAGE}"
echo ""
echo "Note: The BOSH Docker CPI pulls images from inside the container,"
echo "so the image must be in a registry, not just local."
echo ""
