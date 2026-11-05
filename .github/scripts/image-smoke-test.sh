#!/usr/bin/env bash
set -euo pipefail

# Use a local-only name so ibosh cannot resolve or pull a previously published
# image. The test must exercise exactly the image built in this workflow.
candidate=localhost:5000/instant-bosh:ci-smoke
stemcell=ghcr.io/cloudfoundry/ubuntu-noble-stemcell:1.647

# The destroy command removes these named resources. Refuse to touch an
# existing environment if the script is run outside a fresh hosted runner.
if docker container inspect instant-bosh >/dev/null 2>&1 ||
   docker volume inspect instant-bosh-store >/dev/null 2>&1 ||
   docker volume inspect instant-bosh-data >/dev/null 2>&1 ||
   docker network inspect instant-bosh >/dev/null 2>&1; then
  echo 'Existing instant-bosh resources found; refusing to run smoke test' >&2
  exit 1
fi
docker tag ghcr.io/rkoster/instant-bosh:latest "$candidate"

cleanup() {
  local status=$?
  if (( status != 0 )); then
    echo "Director smoke test failed (exit $status)."
    docker ps -a --filter name=instant-bosh || true
    timeout 30 docker logs instant-bosh 2>&1 || true
    if [[ -n "${BOSH_ENVIRONMENT:-}" ]]; then
      timeout 30 bosh tasks --recent=10 || true
      timeout 30 bosh task 1 --debug || true
    fi
  fi
  timeout 60 ibosh docker destroy --force || true
  exit "$status"
}
trap cleanup EXIT

# start's automatic stemcell upload only warns on failure, so skip it and run
# an explicit upload below whose exit status is required for publication.
timeout 10m ibosh docker start --image "$candidate" --skip-stemcell-upload
candidate_id=$(docker image inspect "$candidate" --format '{{.Id}}')
running_id=$(docker inspect instant-bosh --format '{{.Image}}')
if [[ "$candidate_id" != "$running_id" ]]; then
  echo 'Director is not running the freshly built image' >&2
  exit 1
fi
environment=$(ibosh docker print-env)
eval "$environment"
timeout 30 bosh env

# This verifies worker startup, task processing, and Docker CPI access from
# the director, not merely that the director API or host Docker daemon responds.
timeout 10m ibosh docker upload-stemcell "$stemcell"
timeout 30 bosh stemcells --json | jq -e \
  '.Tables[0].Rows[] | select(.name == "bosh-docker-ubuntu-noble" and .version == "1.647")' >/dev/null
