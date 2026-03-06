# Dockerfile for patching instant-bosh director for Apple Silicon
#
# This patches the BOSH director release templates to work under Rosetta
# x86_64 emulation on Apple Silicon Macs.
#
# The main issue is that seccomp filters are architecture-specific and fail
# under Rosetta with "error loading seccomp filter: invalid argument".
#
# Solution:
# 1. Enable privileged mode in BPM configs (disables seccomp)
# 2. Drop privileges back to vcap user in startup scripts (since privileged
#    mode runs processes as root, but most services should run as vcap)
#
# Usage:
#   docker build --build-arg BASE_IMAGE=ghcr.io/rkoster/instant-bosh:sha-xxx \
#                -t my-patched-instant-bosh .
#
# Or use the build.sh script:
#   ./build.sh ghcr.io/rkoster/instant-bosh:sha-xxx

ARG BASE_IMAGE=ghcr.io/rkoster/instant-bosh:latest
FROM ${BASE_IMAGE}

LABEL maintainer="instant-bosh"
LABEL description="instant-bosh patched for Apple Silicon (Rosetta x86_64 emulation)"

# ============================================================================
# STEP 1: Patch BPM templates to enable privileged mode
# ============================================================================
# This disables seccomp filters which fail under Rosetta due to architecture
# mismatch (x86_64 filters on arm64 kernel)
#
# For configs that already have an "unsafe" block, we add "privileged" => true
# inside that block. For configs without "unsafe", we add a new block.

# Patch director bpm.yml template:
# - director_config already has unsafe block with unrestricted_volumes, add privileged there
# - nginx_config, scheduler_config, sync_dns_config, metrics_server_config need new unsafe blocks
RUN sed -i 's/"unrestricted_volumes" => \[{"path" => "\/var\/vcap\/bosh\/etc"}\]/"privileged" => true, "unrestricted_volumes" => [{"path" => "\/var\/vcap\/bosh\/etc"}]/' \
        /var/vcap/jobs/director/templates/bpm.yml && \
    sed -i 's/nginx_config = {/nginx_config = {\n  "unsafe" => { "privileged" => true },/' \
        /var/vcap/jobs/director/templates/bpm.yml && \
    sed -i 's/scheduler_config = {/scheduler_config = {\n  "unsafe" => { "privileged" => true },/' \
        /var/vcap/jobs/director/templates/bpm.yml && \
    sed -i 's/sync_dns_config = {/sync_dns_config = {\n  "unsafe" => { "privileged" => true },/' \
        /var/vcap/jobs/director/templates/bpm.yml && \
    sed -i 's/metrics_server_config = {/metrics_server_config = {\n  "unsafe" => { "privileged" => true },/' \
        /var/vcap/jobs/director/templates/bpm.yml

# Patch postgres bpm.yml template - no existing unsafe block
RUN sed -i 's/postgres_config = {/postgres_config = {\n  "unsafe" => { "privileged" => true },/' \
        /var/vcap/jobs/postgres/templates/bpm.yml

# Patch create-database.erb - use vcap user for oid2name check instead of ${USER}
# When postgres runs as vcap (via su), initdb creates the superuser as 'vcap', not 'postgres'.
# The oid2name check must use vcap to verify postgres is running before the postgres role is created.
RUN sed -i 's/oid2name -H \${HOST} -U \${USER}/oid2name -H ${HOST} -U vcap/' \
        /var/vcap/jobs/postgres/templates/create-database.erb

# Patch nats bpm.yml template:
# - nats_config has no unsafe block, add one
# - bosh_nats_sync already has unsafe block with host_pid_namespace, add privileged there
RUN sed -i 's/nats_config = {/nats_config = {\n  "unsafe" => { "privileged" => true },/' \
        /var/vcap/jobs/nats/templates/bpm.yml && \
    sed -i 's/"host_pid_namespace" => true/"privileged" => true, "host_pid_namespace" => true/' \
        /var/vcap/jobs/nats/templates/bpm.yml

# Patch health_monitor bpm.yml template - already has unsafe block with unrestricted_volumes
RUN sed -i 's/"unrestricted_volumes" => \[/"privileged" => true,\n    "unrestricted_volumes" => [/' \
        /var/vcap/jobs/health_monitor/templates/bpm.yml

# Patch blobstore bpm.yml template - no existing unsafe block
RUN sed -i 's/blobstore_config = {/blobstore_config = {\n  "unsafe" => { "privileged" => true },/' \
        /var/vcap/jobs/blobstore/templates/bpm.yml

# ============================================================================
# STEP 2: Patch startup scripts to drop privileges to vcap user
# ============================================================================
# BPM privileged mode runs processes as root, but most services should run
# as vcap for proper file ownership and security.

# Patch postgres startup - PostgreSQL explicitly refuses to run as root
RUN sed -i 's|^exec \$PACKAGE_DIR/bin/postgres|# Drop privileges to vcap (postgres refuses to run as root)\nif [ "$(id -u)" = "0" ]; then\n    exec su -s /bin/bash vcap -c "exec $PACKAGE_DIR/bin/postgres -h $HOST -p $PORT -D $DATA_DIR"\nfi\nexec $PACKAGE_DIR/bin/postgres|' \
        /var/vcap/jobs/postgres/templates/postgres.erb

# Patch director startup - wait for postgres role and drop to vcap
# The create-database script runs in the background and creates the postgres role.
# We need to wait for it before running migrations.
COPY wait-for-postgres-role.sh /usr/local/bin/wait-for-postgres-role.sh
RUN chmod +x /usr/local/bin/wait-for-postgres-role.sh && \
    sed -i 's|/var/vcap/packages/director/bin/bosh-director-migrate|/usr/local/bin/wait-for-postgres-role.sh\n\n/var/vcap/packages/director/bin/bosh-director-migrate|' \
        /var/vcap/jobs/director/templates/director

# Patch director startup - drop to vcap with environment preserved
RUN sed -i 's|^exec /var/vcap/packages/director/bin/bosh-director|# Drop privileges to vcap when running as root\nif [ "$(id -u)" = "0" ]; then\n    exec su -s /bin/bash vcap -c "source /var/vcap/jobs/director/env \&\& exec /var/vcap/packages/director/bin/bosh-director -c /var/vcap/jobs/director/config/director.yml"\nfi\nexec /var/vcap/packages/director/bin/bosh-director|' \
        /var/vcap/jobs/director/templates/director

# Patch scheduler startup - drop to vcap with environment preserved
RUN sed -i 's|^exec /var/vcap/packages/director/bin/bosh-director-scheduler|# Drop privileges to vcap when running as root\nif [ "$(id -u)" = "0" ]; then\n    exec su -s /bin/bash vcap -c "source /var/vcap/jobs/director/env \&\& exec /var/vcap/packages/director/bin/bosh-director-scheduler -c /var/vcap/jobs/director/config/director.yml"\nfi\nexec /var/vcap/packages/director/bin/bosh-director-scheduler|' \
        /var/vcap/jobs/director/templates/scheduler

# Patch sync-dns startup - drop to vcap with environment preserved
RUN sed -i 's|^exec /var/vcap/packages/director/bin/bosh-director-sync-dns|# Drop privileges to vcap when running as root\nif [ "$(id -u)" = "0" ]; then\n    exec su -s /bin/bash vcap -c "source /var/vcap/jobs/director/env \&\& exec /var/vcap/packages/director/bin/bosh-director-sync-dns -c /var/vcap/jobs/director/config/director.yml"\nfi\nexec /var/vcap/packages/director/bin/bosh-director-sync-dns|' \
        /var/vcap/jobs/director/templates/sync-dns

# Patch metrics-server startup - drop to vcap with environment preserved
RUN sed -i 's|^exec /var/vcap/packages/director/bin/bosh-director-metrics-server|# Drop privileges to vcap when running as root\nif [ "$(id -u)" = "0" ]; then\n    exec su -s /bin/bash vcap -c "source /var/vcap/jobs/director/env \&\& exec /var/vcap/packages/director/bin/bosh-director-metrics-server -c /var/vcap/jobs/director/config/director.yml"\nfi\nexec /var/vcap/packages/director/bin/bosh-director-metrics-server|' \
        /var/vcap/jobs/director/templates/metrics-server

# Patch nats startup script (it's a wrapper that sources env and execs)
RUN if [ -f /var/vcap/jobs/nats/templates/nats.erb ]; then \
        sed -i 's|^exec /var/vcap/packages/nats/bin/nats-server|# Drop privileges to vcap when running as root\nif [ "$(id -u)" = "0" ]; then\n    exec su -s /bin/bash vcap -c "exec /var/vcap/packages/nats/bin/nats-server -c /var/vcap/jobs/nats/config/nats.cfg"\nfi\nexec /var/vcap/packages/nats/bin/nats-server|' \
            /var/vcap/jobs/nats/templates/nats.erb; \
    fi

# Patch bosh_nats_sync startup
RUN if [ -f /var/vcap/jobs/nats/templates/bosh_nats_sync.erb ]; then \
        sed -i 's|^exec /var/vcap/packages/nats/bin/bosh-nats-sync|# Drop privileges to vcap when running as root\nif [ "$(id -u)" = "0" ]; then\n    exec su -s /bin/bash vcap -c "source /var/vcap/packages/director-ruby-3.3/bosh/runtime.env \&\& exec /var/vcap/packages/nats/bin/bosh-nats-sync -c /var/vcap/jobs/nats/config/bosh_nats_sync_config.yml"\nfi\nexec /var/vcap/packages/nats/bin/bosh-nats-sync|' \
            /var/vcap/jobs/nats/templates/bosh_nats_sync.erb; \
    fi

# Patch health_monitor startup
RUN if [ -f /var/vcap/jobs/health_monitor/templates/health_monitor.erb ]; then \
        sed -i 's|^exec /var/vcap/packages/health_monitor/bin/bosh-monitor|# Drop privileges to vcap when running as root\nif [ "$(id -u)" = "0" ]; then\n    exec su -s /bin/bash vcap -c "source /var/vcap/packages/director-ruby-3.3/bosh/runtime.env \&\& exec /var/vcap/packages/health_monitor/bin/bosh-monitor -c /var/vcap/jobs/health_monitor/config/health_monitor.yml"\nfi\nexec /var/vcap/packages/health_monitor/bin/bosh-monitor|' \
            /var/vcap/jobs/health_monitor/templates/health_monitor.erb; \
    fi

# ============================================================================
# STEP 3: Fix nginx tmp directory permissions
# ============================================================================
# Nginx workers run as 'nobody' but the tmp directories are created by BPM as 'vcap'.
# We need to make these directories writable by 'nobody' so file uploads work.
# Patch the director's pre-start script to fix permissions before nginx starts.

RUN sed -i '/^ln -fs \$TMPDIR\/proxy/a\\n# Fix nginx tmp directory permissions for nobody user (nginx worker)\nchmod -R 777 $TMPDIR' \
        /var/vcap/jobs/director/templates/pre-start.erb

# Also fix the nginx upload_store_access to make uploaded files readable by all users
# (nginx runs as nobody but workers run as vcap)
RUN sed -i 's/upload_store_access user:r;/upload_store_access all:rw;/' \
        /var/vcap/jobs/director/templates/nginx.conf.erb

# ============================================================================
# STEP 3b: Fix blobstore directory permissions
# ============================================================================
# Blobstore nginx workers run as 'nobody' but need to write to:
# - /var/vcap/store/blobstore/store/ (blob storage)
# - /var/vcap/data/blobstore/tmp/ (client_body temp files)
# Both directories are owned by vcap. Patch the blobstore pre-start script to fix permissions.

RUN sed -i '/^fi$/a\\n# Fix blobstore directory permissions for nobody user (nginx worker)\nmkdir -p /var/vcap/store/blobstore/store\nchmod -R 777 /var/vcap/store/blobstore\nchmod -R 777 /var/vcap/data/blobstore/tmp' \
        /var/vcap/jobs/blobstore/templates/pre-start

# ============================================================================
# STEP 4: Notes on other processes
# ============================================================================
# - worker_ctl: Already uses chpst to run as RUNAS=vcap, no changes needed
# - nginx: Needs root to bind ports, drops to 'nobody' via its config
# - blobstore nginx: Same as above

# Verify key patches were applied
RUN grep -q "privileged" /var/vcap/jobs/director/templates/bpm.yml && \
    grep -q "privileged" /var/vcap/jobs/postgres/templates/bpm.yml && \
    grep -q "id -u" /var/vcap/jobs/postgres/templates/postgres.erb && \
    grep -q "id -u" /var/vcap/jobs/director/templates/director && \
    echo "All patches verified successfully"
