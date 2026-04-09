# consolidate

This package merges the ~15 cf-deployment instance groups into 5 consolidated
groups suitable for a single-node instant-bosh deployment.

## Consolidated groups

| Group | vm_type | Source instance groups |
|---|---|---|
| `control` | medium | diego-api, uaa, api, cc-worker, scheduler\*, log-cache, **credhub**†, log-api, rotate-cc-database-key |
| `compute` | small-highmem | diego-cell |
| `database` | small | database, **nats**‡ |
| `router` | minimal | router, tcp-router, ssh_proxy\*, **smoke-tests**§ |
| `blobstore` | small | singleton-blobstore, **doppler**¶ |

\* `ssh_proxy` is extracted from `scheduler` and placed in `router`.  
† `credhub` is in `control` (not `database`) because credhub's start script (`wait_for_uaa`) requires UAA to be up first; UAA is also in `control`.  
‡ `nats` is in `database` (not `control`) to avoid a `pid_utils` package name collision between the `nats` and `diego` releases.  
§ `smoke-tests` is in `router` (not `control`) to avoid a `golang-1-linux` package name collision between the `cf-smoke-tests` and `capi` releases.  
¶ `doppler` is in `blobstore` (not `control`) because both `doppler` and `reverse_log_proxy` (from `log-api`) default to port 8082; keeping them on separate VMs avoids the bind conflict.

## BOSH package name collisions

BOSH rejects a deployment if two jobs colocated on the same instance group depend
on packages that share a name but have different content (fingerprint). This is a
hard constraint enforced at deploy time.

### How to detect collisions

Use `bosh inspect-release` and `jq` to extract the package name and fingerprint
for every package in every release. Then look for names that appear with more than
one distinct fingerprint across all releases colocated in the same group.

**Step 1 — list current release versions:**

```bash
bosh releases --json | jq -r '
  .Tables[0].Rows[] | select(.version | endswith("*")) |
  "\(.name)/\(.version[:-1])"
'
```

**Step 2 — extract packages for a set of releases:**

```bash
for rel in routing/0.368.0 diego/2.130.0 loggregator-agent/8.3.15; do
  rname="${rel%/*}"
  bosh inspect-release "$rel" --json | jq -r --arg rname "$rname" '
    .Tables[] | select(.Content == "packages") | .Rows[] |
    (.package | split("/")) as $parts |
    "\($parts[0]) \($parts[1][:16]) \($rname)"
  ' | sort -u
done
```

**Step 3 — find collisions (same package name, different fingerprint):**

```bash
# Collect all "package fingerprint release" lines into a temp file
for rel in routing/0.368.0 diego/2.130.0 loggregator-agent/8.3.15; do
  rname="${rel%/*}"
  bosh inspect-release "$rel" --json | jq -r --arg rname "$rname" '
    .Tables[] | select(.Content == "packages") | .Rows[] |
    (.package | split("/")) as $parts |
    "\($parts[0]) \($parts[1][:16]) \($rname)"
  ' | sort -u
done | awk '{print $1, $2}' | sort -u | awk '{print $1}' | sort | uniq -d
```

Any package name printed is a collision — two or more releases provide packages
with the same name but different fingerprints. Jobs from those releases cannot be
colocated on the same instance group.

### Known collisions in cf-deployment

| Package | Release A | Release B | Resolution |
|---|---|---|---|
| `pid_utils` | `nats` | `diego` | Move `nats` source group to `database` |
| `golang-1-linux` | `capi` | `cf-smoke-tests` | Move `smoke-tests` source group to `router` |

### What is safe to share

Packages with the **same** fingerprint across multiple releases are safe — BOSH
deduplicates them automatically. Common examples across CF releases:

- `golang-1.25-linux` — shared across `diego`, `routing`, `loggregator-agent`, `silk`, `cf-networking`, `log-cache`, `statsd-injector`, `loggregator`
- `cf-cli-8-linux` — shared between `routing` and `cf-cli`
- `golang-1-linux` — shared between `capi` and `pxc` (same fingerprint)

### Checking when cf-deployment upgrades

When updating cf-deployment or any release version, re-run the collision check
for the full set of releases in each consolidated group to ensure no new
collisions have been introduced:

```bash
# Control group releases (adjust versions as needed)
CONTROL_RELEASES="diego/2.130.0 silk/3.105.0 loggregator-agent/8.3.15 uaa/78.8.0 \
  routing/0.368.0 statsd-injector/1.11.52 capi/1.229.0 cf-networking/3.105.0 \
  log-cache/3.2.5 loggregator/107.0.25 binary-buildpack/1.1.21 \
  dotnet-core-buildpack/2.4.48 go-buildpack/1.10.44 java-buildpack/4.77.0 \
  nodejs-buildpack/1.8.44 nginx-buildpack/1.2.35 r-buildpack/1.2.26 \
  php-buildpack/5.0.1 python-buildpack/1.8.43 ruby-buildpack/1.10.29 \
  staticfile-buildpack/1.6.35"

(for rel in $CONTROL_RELEASES; do
  rname="${rel%/*}"
  bosh inspect-release "$rel" --json | jq -r --arg rname "$rname" '
    .Tables[] | select(.Content == "packages") | .Rows[] |
    (.package | split("/")) as $parts |
    "\($parts[0]) \($parts[1][:16]) \($rname)"
  ' | sort -u
done) | awk '{print $1, $2}' | sort -u | awk '{print $1}' | sort | uniq -d
```

No output means no collisions. Any printed package name requires attention.
