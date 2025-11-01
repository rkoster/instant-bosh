.PHONY: help build dev-bob-build run stop logs sync clean reset

# Default target
help:
	@echo "Available targets:"
	@echo "  sync           - Sync vendored dependencies using vendir"
	@echo "  build          - Build BOSH OCI image using bob"
	@echo "  dev-bob-build  - Build BOSH OCI image using go run (development)"
	@echo "  run            - Run the built BOSH image using docker run"
	@echo "  stop           - Stop the running BOSH container"
	@echo "  logs           - Show logs from the running BOSH container"
	@echo "  clean          - Stop container and remove image (keeps volumes)"
	@echo "  reset          - Full reset: stop container, remove volumes and image"
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
	docker run -d --name instant-bosh --rm --privileged \
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
	@echo "Waiting for BOSH to be ready..."
	@START_TIME=$$(date +%s); \
	MAX_WAIT=300; \
	until curl -k -s -f https://127.0.0.1:25555/info > /dev/null 2>&1; do \
		CURRENT_TIME=$$(date +%s); \
		ELAPSED=$$((CURRENT_TIME - START_TIME)); \
		if [ $$ELAPSED -gt $$MAX_WAIT ]; then \
			echo ""; \
			echo "Timeout waiting for BOSH to start. Showing logs:"; \
			docker logs instant-bosh; \
			exit 1; \
		fi; \
		if ! docker ps | grep -q instant-bosh; then \
			echo ""; \
			echo "Container stopped unexpectedly. Last logs:"; \
			docker logs instant-bosh 2>&1 || echo "Container already removed"; \
			exit 1; \
		fi; \
		echo -n "."; \
		sleep 2; \
	done; \
	END_TIME=$$(date +%s); \
	ELAPSED=$$((END_TIME - START_TIME)); \
	echo ""; \
	echo "BOSH is ready! (took $${ELAPSED}s)"; \
	curl -k -s https://127.0.0.1:25555/info | jq -r '"Name: " + .name + "\nUUID: " + .uuid'

# Stop the running BOSH container
stop:
	docker stop instant-bosh || true

# Show logs from the running BOSH container
logs:
	docker logs -f instant-bosh

# Debug: render templates only without running hooks
debug:
	docker run --rm \
		-v /var/run/docker.sock:/var/run/docker.sock \
		instant-bosh:latest \
		--template-only --print-rendered-templates

# Clean up container and image (keeps volumes for faster restart)
clean:
	docker stop instant-bosh 2>/dev/null || true
	docker rmi instant-bosh:latest 2>/dev/null || true
	@echo "Stopped container and removed image (volumes preserved)"

# Full reset: remove everything including volumes
reset:
	docker stop instant-bosh 2>/dev/null || true
	docker rmi instant-bosh:latest 2>/dev/null || true
	docker volume rm instant-bosh-store instant-bosh-data 2>/dev/null || true
	@echo "Full reset complete (container, image, and volumes removed)"
