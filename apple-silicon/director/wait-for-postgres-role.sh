#!/bin/bash
# Wait for postgres role to be created
# The create-database script runs in the background and creates the postgres role.
# We need to wait for it before running migrations.

echo "Waiting for postgres role to be created..."
for i in $(seq 1 60); do
  if /var/vcap/packages/postgres-15/bin/psql -h 127.0.0.1 -U vcap -d postgres -tAc "SELECT 1 FROM pg_roles WHERE rolname='postgres'" 2>/dev/null | grep -q 1; then
    echo "postgres role found"
    exit 0
  fi
  echo "Waiting for postgres role... ($i/60)"
  sleep 1
done

echo "ERROR: postgres role not found after 60 seconds"
exit 1
