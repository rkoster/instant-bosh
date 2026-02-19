#!/bin/bash
# Script to reduce BOSH director max_threads to help with cleaner CF deploys
# Usage: ./scripts/reduce-max-threads.sh [max_threads] [project] [instance] [restart_workers]
#
# Arguments:
#   max_threads     - Number of max concurrent threads (default: 4)
#   project         - Incus project name (default: ibosh)
#   instance        - Incus instance name (default: instant-bosh)
#   restart_workers - Also restart DJ workers: yes/no (default: no)

set -euo pipefail

MAX_THREADS="${1:-4}"
PROJECT="${2:-ibosh}"
INSTANCE="${3:-instant-bosh}"
RESTART_WORKERS="${4:-no}"
CONFIG_PATH="/var/vcap/jobs/director/config/director.yml"
BPM="/var/vcap/packages/bpm/bin/bpm"

echo "==> Reducing BOSH director max_threads to ${MAX_THREADS}"
echo "    Project: ${PROJECT}"
echo "    Instance: ${INSTANCE}"
echo "    Restart workers: ${RESTART_WORKERS}"

# Read current config, update max_threads
echo "==> Updating director.yml..."
incus exec "${INSTANCE}" --project "${PROJECT}" -- bash -c "
  sed -i 's/\"max_threads\":[0-9]*/\"max_threads\":${MAX_THREADS}/' ${CONFIG_PATH}
"

# Verify the change
echo "==> Verifying change..."
incus exec "${INSTANCE}" --project "${PROJECT}" -- bash -c "
  grep -o '\"max_threads\":[0-9]*' ${CONFIG_PATH}
"

# Optionally restart workers
if [[ "${RESTART_WORKERS}" == "yes" ]]; then
  echo "==> Stopping director workers..."
  incus exec "${INSTANCE}" --project "${PROJECT}" -- bash -c '
    for pid in $(pgrep -x ruby); do
      if ps -p $pid -o args= 2>/dev/null | grep -q bosh-director-worker; then
        kill -9 $pid 2>/dev/null || true
      fi
    done
    rm -f /var/vcap/sys/run/director/worker_*.pid 2>/dev/null || true
  '
fi

# Restart director processes via BPM
echo "==> Restarting director processes via BPM..."
incus exec "${INSTANCE}" --project "${PROJECT}" -- bash -c "
  ${BPM} stop director -p sync_dns || true
  ${BPM} stop director -p scheduler || true  
  ${BPM} stop director || true
  ${BPM} start director
  ${BPM} start director -p scheduler
  ${BPM} start director -p sync_dns
"

# Optionally start workers
if [[ "${RESTART_WORKERS}" == "yes" ]]; then
  echo "==> Starting director workers..."
  incus exec "${INSTANCE}" --project "${PROJECT}" -- bash -c "
    for i in 1 2 3 4; do
      nohup /var/vcap/jobs/director/bin/worker_ctl start \$i </dev/null >/dev/null 2>&1 &
    done
  "
fi

echo "==> Waiting for director to start..."
sleep 3

# Verify processes are back
echo "==> Checking BPM status..."
incus exec "${INSTANCE}" --project "${PROJECT}" -- ${BPM} list

echo "==> Checking workers..."
incus exec "${INSTANCE}" --project "${PROJECT}" -- bash -c "
  ps -o pid,start,cmd -C ruby 2>/dev/null | grep bosh-director-worker || echo 'No workers found'
"

echo "==> Done! Director now running with max_threads=${MAX_THREADS}"
