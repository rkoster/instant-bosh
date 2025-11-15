.PHONY: help build build-ibosh dev-bob-build run stop logs sync clean reset print-env

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

# Build BOSH OCI image
build:
	bob build \
		--manifest vendor/bosh-deployment/bosh.yml \
		--ops-file vendor/bosh-deployment/docker/cpi.yml \
		--ops-file vendor/bosh-deployment/docker/unix-sock.yml \
		--ops-file ops/stemcell.yml \
		--ops-file ops/docker-localhost.yml \
                --ops-file ops/fast-nats-sync.yml \
		--ops-file ops/disable-short-lived-nats-credentials.yml \
		--ops-file vendor/bosh-deployment/jumpbox-user.yml \
		--ops-file ops/pre-start-setup.yml \
		--ops-file ops/embed-license.yml \
		--output ghcr.io/rkoster/instant-bosh:latest

# Build BOSH OCI image using development version of bob
dev-bob-build:
	cd ../bosh-oci-builder && DOCKER_HOST=unix://$(HOME)/.config/colima/default/docker.sock go run ./cmd/bob build \
		--manifest ../instant-bosh/vendor/bosh-deployment/bosh.yml \
		--ops-file ../instant-bosh/vendor/bosh-deployment/docker/cpi.yml \
		--ops-file ../instant-bosh/vendor/bosh-deployment/docker/unix-sock.yml \
		--ops-file ../instant-bosh/ops/stemcell.yml \
		--ops-file ../instant-bosh/ops/docker-localhost.yml \
                --ops-file ../instant-bosh/ops/fast-nats-sync.yml \
		--ops-file ../instant-bosh/ops/disable-short-lived-nats-credentials.yml \
		--ops-file ../instant-bosh/vendor/bosh-deployment/jumpbox-user.yml \
		--ops-file ../instant-bosh/ops/pre-start-setup.yml \
		--ops-file ../instant-bosh/ops/embed-license.yml \
		--output ghcr.io/rkoster/instant-bosh:latest

# Run the built BOSH image
run:
	docker volume create instant-bosh-store || true
	docker volume create instant-bosh-data || true
	docker network create instant-bosh --subnet=10.245.0.0/16 --gateway=10.245.0.1 || true
	docker run -d --name instant-bosh --rm --privileged \
		--network instant-bosh \
		--ip 10.245.0.10 \
		-p 25555:25555 \
		-p 2222:22 \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v instant-bosh-store:/var/vcap/store \
		-v instant-bosh-data:/var/vcap/data \
		instant-bosh:latest \
		-v internal_ip=10.245.0.10 \
		-v internal_cidr=10.245.0.0/16 \
		-v internal_gw=10.245.0.1 \
		-v director_name=instant-bosh \
		-v network=instant-bosh
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
		printf "."; \
		sleep 2; \
	done; \
	END_TIME=$$(date +%s); \
	ELAPSED=$$((END_TIME - START_TIME)); \
	echo ""; \
	echo "BOSH is ready! (took $${ELAPSED}s)"; \
	curl -k -s https://127.0.0.1:25555/info | jq -r '"Name: " + .name + "\nUUID: " + .uuid'
	@echo ""
	@echo "Updating cloud config..."
	@ADMIN_PASSWORD=$$(docker exec instant-bosh cat /var/vcap/store/vars-store.yml 2>/dev/null | bosh int - --path=/admin_password); \
	DIRECTOR_CERT=$$(docker exec instant-bosh cat /var/vcap/store/vars-store.yml 2>/dev/null | bosh int - --path=/director_ssl/ca); \
	BOSH_CLIENT=admin \
	BOSH_CLIENT_SECRET=$$ADMIN_PASSWORD \
	BOSH_ENVIRONMENT=https://127.0.0.1:25555 \
	BOSH_CA_CERT=$$DIRECTOR_CERT \
	bosh update-cloud-config vendor/bosh-deployment/docker/cloud-config.yml -v network=instant-bosh -n
	@echo "Cloud config updated successfully!"
	@echo ""
	@echo "Uploading os-conf-release for VM SSH support..."
	@ADMIN_PASSWORD=$$(docker exec instant-bosh cat /var/vcap/store/vars-store.yml 2>/dev/null | bosh int - --path=/admin_password); \
	DIRECTOR_CERT=$$(docker exec instant-bosh cat /var/vcap/store/vars-store.yml 2>/dev/null | bosh int - --path=/director_ssl/ca); \
	BOSH_CLIENT=admin \
	BOSH_CLIENT_SECRET=$$ADMIN_PASSWORD \
	BOSH_ENVIRONMENT=https://127.0.0.1:25555 \
	BOSH_CA_CERT=$$DIRECTOR_CERT \
	bosh upload-release https://bosh.io/d/github.com/cloudfoundry/os-conf-release
	@echo "os-conf-release uploaded successfully!"
	@echo ""
	@echo "Updating runtime config to enable SSH on VMs..."
	@ADMIN_PASSWORD=$$(docker exec instant-bosh cat /var/vcap/store/vars-store.yml 2>/dev/null | bosh int - --path=/admin_password); \
	DIRECTOR_CERT=$$(docker exec instant-bosh cat /var/vcap/store/vars-store.yml 2>/dev/null | bosh int - --path=/director_ssl/ca); \
	BOSH_CLIENT=admin \
	BOSH_CLIENT_SECRET=$$ADMIN_PASSWORD \
	BOSH_ENVIRONMENT=https://127.0.0.1:25555 \
	BOSH_CA_CERT=$$DIRECTOR_CERT \
	bosh update-runtime-config runtime-config-enable-vm-ssh.yml -n
	@echo "Runtime config updated successfully!"

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

# Print environment variables for BOSH CLI
print-env:
	@if ! docker ps --format '{{.Names}}' | grep -q '^instant-bosh$$'; then \
		echo "Error: instant-bosh container is not running. Please run 'make run' first." >&2; \
		exit 1; \
	fi
	@ADMIN_PASSWORD=$$(docker exec instant-bosh cat /var/vcap/store/vars-store.yml 2>/dev/null | bosh int - --path=/admin_password); \
	DIRECTOR_CERT=$$(docker exec instant-bosh cat /var/vcap/store/vars-store.yml 2>/dev/null | bosh int - --path=/director_ssl/ca); \
	JUMPBOX_KEY=$$(docker exec instant-bosh cat /var/vcap/store/vars-store.yml 2>/dev/null | bosh int - --path=/jumpbox_ssh/private_key); \
	JUMPBOX_KEY_FILE=$$(mktemp /tmp/jumpbox-key.XXXXXX); \
	echo "$$JUMPBOX_KEY" > $$JUMPBOX_KEY_FILE; \
	chmod 600 $$JUMPBOX_KEY_FILE; \
	echo "export BOSH_CLIENT=admin"; \
	echo "export BOSH_CLIENT_SECRET=$$ADMIN_PASSWORD"; \
	echo "export BOSH_ENVIRONMENT=https://127.0.0.1:25555"; \
	echo "export BOSH_CA_CERT='$$DIRECTOR_CERT'"; \
	echo "export BOSH_ALL_PROXY=ssh+socks5://jumpbox@localhost:2222?private-key=$$JUMPBOX_KEY_FILE"
