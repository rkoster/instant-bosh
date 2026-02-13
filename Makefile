.PHONY: help build build-ibosh dev-bob-build run stop logs sync clean reset print-env

# Common ops-paths (paths relative to repo root)
BOB_OPS = \
	--ops-file manifests/bosh-deployment/docker/cpi.yml \
	--ops-file manifests/bosh-deployment/docker/unix-sock.yml \
	--ops-file ops/lxd-cpi-release.yml \
	--ops-file ops/stemcell.yml \
	--ops-file ops/docker-localhost.yml \
	--ops-file ops/fast-nats-sync.yml \
	--ops-file ops/pre-start-setup.yml \
	--ops-file ops/disable-short-lived-nats-credentials.yml \
	--ops-file manifests/bosh-deployment/jumpbox-user.yml \
	--ops-file manifests/uaa-lite-release/operations/uaa-lite.yml \
	--ops-file manifests/bosh-deployment/misc/config-server.yml

# Generate bob --ops-file args for dev-bob-build (prefix with ../instant-bosh/)
DEV_PREFIX = ../instant-bosh/
DEV_BOB_OPS = $(foreach p,$(OPS_PATHS),--ops-file $(DEV_PREFIX)$(p))

# Rebuild trigger: bob underscore prefix fix (issue #100) - attempt 2
help:
	@echo "Available targets:"
	@echo "  sync           - Sync vendored dependencies using vendir"
	@echo "  build          - Build BOSH OCI image using bob"
	@echo "  build-ibosh    - Build the ibosh CLI"
	@echo "  dev-bob-build  - Build BOSH OCI image using go run (development)"
	@echo "  run            - Run the built BOSH image using docker run (deprecated: use ibosh start)"
	@echo "  stop           - Stop the running BOSH container (deprecated: use ibosh stop)"
	@echo "  logs           - Show logs from the running BOSH container"
	@echo "  print-env      - Print environment variables for BOSH CLI (use: eval \"\$$(make print-env)\")"
	@echo "  clean          - Stop container and remove image (keeps volumes)"
	@echo "  reset          - Full reset: stop container, remove volumes and image"
	@echo "  help           - Show this help message"

# Sync vendored dependencies
sync:
	devbox run vendir sync

build-ibosh:
	devbox run build-ibosh

check-manifest:
	@bosh int $(BOB_OPS) manifests/bosh-deployment/bosh.yml --vars-store=/tmp/creds.yml; rm /tmp/creds.yml \

# Build BOSH OCI image
build:
	bob build \
		--manifest manifests/bosh-deployment/bosh.yml \
		$(BOB_OPS) \
		--embed-ops-file ops/director-alternative-names.yml \
		--embed-ops-file ops/lxd-cpi.yml \
		--license LICENSE \
		--output ghcr.io/rkoster/instant-bosh:latest

# Build BOSH OCI image using development version of bob
dev-bob-build:
	cd ../bosh-oci-builder && DOCKER_HOST=unix://$(HOME)/.config/colima/default/docker.sock \
		go run ./cmd/bob build \
		--manifest manifests/bosh-deployment/bosh.yml \
		$(BOB_OPS) \
		--embed-ops-file ../instant-bosh/ops/director-alternative-names.yml \
		--embed-ops-file ../instant-bosh/ops/lxd-cpi.yml \
		--license ../instant-bosh/LICENSE \
		--output ghcr.io/rkoster/instant-bosh:latest
