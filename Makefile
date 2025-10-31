.PHONY: help build dev-bob-build run sync clean

# Default target
help:
	@echo "Available targets:"
	@echo "  sync           - Sync vendored dependencies using vendir"
	@echo "  build          - Build BOSH OCI image using bob"
	@echo "  dev-bob-build  - Build BOSH OCI image using go run (development)"
	@echo "  run            - Run the built BOSH image using docker run"
	@echo "  clean          - Remove built images (cleanup)"
	@echo "  help           - Show this help message"

# Sync vendored dependencies
sync:
	devbox run vendir sync

# Build BOSH OCI image
build:
	bob build \
		--manifest vendor/bosh-deployment/bosh.yml \
		--ops-file vendor/bosh-deployment/docker/cpi.yml \
		--ops-file vendor/bosh-deployment/docker/unix-sock.yml \
		--ops-file ops-stemcell.yml \
		--vars-file vars.yml \
		--output instant-bosh:latest

# Build BOSH OCI image using development version of bob
dev-bob-build:
	cd ../bosh-oci-builder && go run ./cmd/bob build \
		--manifest ../instant-bosh/vendor/bosh-deployment/bosh.yml \
		--ops-file ../instant-bosh/vendor/bosh-deployment/docker/cpi.yml \
		--ops-file ../instant-bosh/vendor/bosh-deployment/docker/unix-sock.yml \
		--ops-file ../instant-bosh/ops-stemcell.yml \
		--vars-file ../instant-bosh/vars.yml \
		--output instant-bosh:latest

# Run the built BOSH image
run:
	docker volume create instant-bosh-store || true
	docker volume create instant-bosh-data || true
	docker run --rm --privileged \
		-p 25555:25555 \
		-e BOSH_INTERNAL_IP=127.0.0.1 \
		-e BOSH_INTERNAL_CIDR=127.0.0.1/32 \
		-e BOSH_INTERNAL_GW=127.0.0.1 \
		-e BOSH_DIRECTOR_NAME=bosh-director \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v instant-bosh-store:/var/vcap/store \
		-v instant-bosh-data:/var/vcap/data \
		instant-bosh:latest \
		--vars-env BOSH

# Debug: render templates only without running hooks
debug:
	docker run --rm \
		-v /var/run/docker.sock:/var/run/docker.sock \
		instant-bosh:latest \
		--template-only --print-rendered-templates

# Clean up Docker volumes
clean:
	docker volume rm instant-bosh-store instant-bosh-data 2>/dev/null || true
	@echo "Cleaned up Docker volumes"
