# Launching Cloud Foundry Stemcells in Incus

This document captures research findings on how to launch Cloud Foundry stemcells as Incus containers.

## Overview

Cloud Foundry stemcells are available as OCI images from GHCR (GitHub Container Registry). These images are designed for use with the Docker CPI and contain a full Ubuntu system with systemd. However, launching them in Incus requires specific configuration to work correctly.

## Image Source

Stemcells are available at:
```
ghcr.io/cloudfoundry/ubuntu-noble-stemcell:<version>
```

Example: `ghcr.io/cloudfoundry/ubuntu-noble-stemcell:1.165`

## Setup Requirements

### 1. Add GHCR as OCI Remote

Incus needs GHCR configured as an OCI remote:

```bash
incus remote add oci-ghcr https://ghcr.io --protocol=oci
```

### 2. Configure Default Profile with Root Disk

The default profile must have a root disk device configured:

```bash
incus storage list  # Find available storage pool (e.g., "local")
incus profile device add default root disk path=/ pool=local
```

## Issues Encountered

### Issue 1: OCI Image Type

When Incus imports OCI images, it marks them as application containers (`CONTAINER (APP)`) rather than system containers. This is stored in:

- Image property: `type: oci`
- Container config: `volatile.container.oci: "true"`
- Additional OCI config: `oci.entrypoint`, `oci.cwd`, `oci.uid`, `oci.gid`

**Impact**: The container runs the OCI entrypoint (`sh`) instead of systemd as PID 1.

### Issue 2: resolv.conf Symlink

The stemcell image has `/etc/resolv.conf` as a symlink (typical for systemd-resolved systems). Incus tries to bind-mount its own resolv.conf over this path, but fails due to the symlink:

```
ERROR: Too many levels of symbolic links - resolv.conf in /opt/incus/lib/lxc/rootfs/etc/resolv.conf was a symbolic link!
```

**Impact**: Container fails to start with "Failed to setup mount entries".

### Issue 3: /run Mount Failure

The container logs also show:
```
ERROR: Failed to mount "none" onto "/opt/incus/lib/lxc/rootfs/run"
```

This appears to be a secondary issue that resolves once the resolv.conf problem is fixed.

## Solution

### Quick Fix (for existing OCI image)

1. Create the container without starting:
   ```bash
   incus init oci-ghcr:cloudfoundry/ubuntu-noble-stemcell:1.165 stemcell-container -s local
   ```

2. Remove the problematic resolv.conf symlink:
   ```bash
   incus file delete stemcell-container/etc/resolv.conf
   ```

3. Start the container:
   ```bash
   incus start stemcell-container
   ```

At this point, the container runs but still as an APP container (using `sh` as entrypoint, not systemd).

### Full Solution (System Container with systemd)

To run the stemcell as a proper system container with systemd:

1. Launch the OCI image and fix resolv.conf (as above)

2. Stop the container:
   ```bash
   incus stop stemcell-container
   ```

3. Publish as a new native Incus image:
   ```bash
   incus publish stemcell-container --alias stemcell-system
   ```

4. Delete the old container and launch from the new image:
   ```bash
   incus delete stemcell-container
   incus launch stemcell-system stemcell-container -s local
   ```

The new image has no OCI metadata, so the container runs systemd as PID 1.

### Verification

Check container type:
```bash
incus list
```

- `CONTAINER (APP)` = OCI/application container (runs entrypoint)
- `CONTAINER` = System container (runs init/systemd)

Verify systemd is running:
```bash
incus exec stemcell-container -- systemctl status
```

## Automation Considerations

For a CPI implementation, the workflow would be:

1. **One-time setup**: Import and convert the stemcell image
   ```bash
   # Import OCI image
   incus image copy oci-ghcr:cloudfoundry/ubuntu-noble-stemcell:1.165 local: --alias stemcell-oci-temp
   
   # Create temporary container
   incus init stemcell-oci-temp stemcell-temp -s local
   
   # Fix resolv.conf
   incus file delete stemcell-temp/etc/resolv.conf
   
   # Publish as system image
   incus publish stemcell-temp --alias stemcell-1.165
   
   # Cleanup
   incus delete stemcell-temp
   incus image delete stemcell-oci-temp
   ```

2. **Per-VM creation**: Launch from the converted image
   ```bash
   incus launch stemcell-1.165 <instance-name> -s local
   ```

## Alternative Approaches to Investigate

1. **Modify image metadata before import**: Edit the OCI image manifest to remove entrypoint/cmd before importing

2. **Use raw.lxc configuration**: Override LXC settings to force systemd execution

3. **Pre-process stemcell images**: Build a conversion pipeline that creates native Incus images from OCI stemcells

## Network Configuration

Note: The containers launched in this research had no network attached. For production use:

```bash
# Create a network if needed
incus network create incusbr0

# Attach to container
incus network attach incusbr0 stemcell-container eth0
```

Or add to default profile:
```bash
incus profile device add default eth0 nic network=incusbr0
```

## References

- [Incus documentation](https://linuxcontainers.org/incus/docs/main/)
- [Cloud Foundry Stemcells](https://bosh.io/docs/stemcell/)
- [BOSH Docker CPI](https://github.com/cloudfoundry/bosh-docker-cpi-release)
