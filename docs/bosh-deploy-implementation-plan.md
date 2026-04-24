# Implementation Plan: `ibosh bosh deploy`

## Overview

Implement a command to deploy a BOSH director as a BOSH deployment (not `create-env`) on an existing instant-bosh director, primarily for BOSH director development workflows.

## Requirements Summary

1. ✅ Deploy BOSH director via `bosh deploy` (not `bosh create-env`)
2. ✅ Use `misc/bosh-dev.yml` ops file to convert create-env manifest format
3. ✅ Default to warden CPI (bosh-lite with Garden RunC)
4. ✅ Automatically apply warden cloud-config
5. ✅ Use config-server for credentials (no vars-store file)
6. ✅ Deployment name format: `bosh-<cpi>` (e.g., `bosh-warden`)
7. ✅ Add `--cpi` flag as extension point (only warden supported initially)
8. ✅ Stemcell assumed already uploaded via `ibosh incus upload-stemcell`
9. ✅ Extract IP selection logic into reusable helper

## Key Design Decisions

### 1. Cloud-Config Strategy
**Decision:** Automatically apply warden cloud-config when deploying with `--cpi warden`

### 2. Credentials Storage
**Decision:** Use instant-bosh config-server (no vars-store file needed)
- Credentials stored at: `/instant-bosh/bosh-<cpi>/<var-name>`
- Example: `/instant-bosh/bosh-warden/admin_password`

### 3. Deployment Naming
**Decision:** Use `bosh-<cpi>` as deployment name
- Format: `bosh-warden`, `bosh-docker` (future), `bosh-incus` (future)
- Allows multiple directors with different CPIs

### 4. CPI Support
**Decision:** Implement only warden CPI first, but add `--cpi` flag as extension point
- Initial: `--cpi warden` (default and only supported value)
- Future: `--cpi docker`, `--cpi incus`

### 5. Stemcell Handling
**Decision:** Assume stemcell already uploaded via `ibosh incus upload-stemcell`
- No stemcell check in deployment command
- BOSH will fail with clear error if stemcell missing

### 6. IP Selection
**Decision:** Extract IP selection logic into reusable helper
- Share between CF and BOSH deployments
- Support network filtering and CIDR range preferences

---

## Phase 1: Infrastructure & Reusable Utilities

### 1.1 Update `vendir.yml`

Add to `manifests/bosh-deployment` section:

```yaml
- path: manifests/bosh-deployment
  git:
    url: https://github.com/cloudfoundry/bosh-deployment/
    ref: origin/master
  includePaths:
    - bosh.yml
    - bosh-lite.yml          # NEW
    - warden/cpi.yml         # NEW
    - warden/cloud-config.yml # NEW
    - docker/cpi.yml
    - docker/unix-sock.yml
    - jumpbox-user.yml
    - uaa.yml                # NEW
    - credhub.yml            # NEW
    - misc/config-server.yml
    - misc/bosh-dev.yml      # NEW - KEY FILE
```

**Action:** Run `vendir sync` after updating

### 1.2 Create `internal/commands/ipselection.go`

**Purpose:** Reusable IP selection logic for both CF and BOSH deployments

**Key Functions:**
- `SelectAvailableIP(ui UI, opts IPSelectionOptions) (string, error)`
- `extractStaticIPsFromCloudConfig(cloudConfigYAML, networkName string) ([]string, error)`
- `filterIPsByRange(ips []string, cidrRange string) []string`
- `parseIPRange(rangeStr string) ([]string, error)` - Extracted from cf.go
- `getUsedIPs() (map[string]bool, error)` - Extracted from cf.go

**Structure:**
```go
type IPSelectionOptions struct {
    NetworkName    string   // Network name to search in cloud-config
    PreferredRange string   // Preferred IP range (e.g., "10.244.0.0/24" for warden)
}
```

**Usage Example:**
```go
// For BOSH warden deployment
opts := IPSelectionOptions{
    NetworkName:    "default",
    PreferredRange: "10.244.0.0/24", // Warden network range
}
ip, err := SelectAvailableIP(ui, opts)
```

### 1.3 Update `internal/commands/cf.go`

**Changes:** Refactor to use new IP selection helper

```go
// In selectRouterIP function, replace inline logic with:
func selectRouterIP(ui UI) (string, error) {
    opts := IPSelectionOptions{
        NetworkName: "default",
        // CF doesn't need preferred range, uses any available IP
    }
    return SelectAvailableIP(ui, opts)
}
```

**Impact:**
- Remove inline IP parsing and selection logic
- Simplify code by ~50 lines
- Enable reuse in BOSH deployment

---

## Phase 2: BOSH Deployment Manifests

### 2.1 Create `internal/manifests/bosh_manifests.go`

**Purpose:** Helper functions to access BOSH deployment manifests

**Functions:**

```go
// BOSHDeploymentManifest returns the main bosh.yml manifest
func BOSHDeploymentManifest() ([]byte, error)

// BOSHOpsFile returns a specific ops file from bosh-deployment
func BOSHOpsFile(name string) ([]byte, error)

// BOSHWardenCloudConfig returns the warden cloud-config
func BOSHWardenCloudConfig() ([]byte, error)

// StandardBOSHOpsFiles returns ops files for deploying BOSH director
// cpiType: currently only "warden" is supported
func StandardBOSHOpsFiles(cpiType string) ([][]byte, error)
```

**Ops Files Order (Warden CPI):**
1. `bosh-lite.yml` - Adds garden-runc, warden CPI, systemd support
2. `warden/cpi.yml` - Warden-specific CPI configuration
3. `jumpbox-user.yml` - SSH jumpbox user
4. `uaa.yml` - UAA authentication
5. `credhub.yml` - CredHub for credentials
6. `misc/bosh-dev.yml` - **KEY:** Convert create-env to deploy format

---

## Phase 3: BOSH Deploy Command

### 3.1 Create `internal/commands/bosh.go`

**Purpose:** Main BOSH deployment logic

#### Data Structures

```go
type BOSHDeployOptions struct {
    CPI        string // CPI type (currently only "warden" supported)
    DirectorIP string // Optional: specify director IP
    DryRun     bool   // Show what would be deployed without deploying
}

type BOSHManifestConfig struct {
    CPI            string
    DirectorIP     string
    Network        string // Network name from cloud-config
    DeploymentName string // Full deployment name (e.g., "bosh-warden")
}

type BOSHManifestFiles struct {
    TmpDir       string
    ManifestPath string
    OpsPaths     []string
}

type BOSHDeleteOptions struct {
    CPI   string // CPI type to determine deployment name
    Force bool   // Skip confirmation
}
```

#### Main Functions

**`BOSHDeployAction(ui UI, opts BOSHDeployOptions) error`**

Main deployment flow:

1. **Prerequisites Check:**
   - Verify BOSH environment variables set
   - Validate CPI type (only "warden" supported)
   - Detect instant-bosh CPI and warn if Docker (needs Incus)

2. **Cloud-Config Setup:**
   - Call `ensureWardenCloudConfig(ui)` to auto-apply warden cloud-config

3. **Configuration Resolution:**
   - Call `ResolveBOSHConfig(ui, opts.CPI, opts.DirectorIP)`
   - Auto-select IP if not provided (from 10.244.0.0/24 range)

4. **Manifest Preparation:**
   - Call `PrepareBOSHManifestFiles(opts.CPI)`
   - Creates temp directory with bosh.yml and all ops files

5. **Manifest Interpolation:**
   - Call `interpolateBOSHManifest(files, config)`
   - Set variables: `internal_ip`, `garden_host`, etc.

6. **Deployment:**
   - Call `deployBOSHDirector(interpolatedPath, config.DeploymentName)`
   - Run `bosh deploy -d bosh-warden manifest.yml -n`

7. **Success Message:**
   - Print targeting instructions with config-server credentials

**`ResolveBOSHConfig(ui UI, cpiType, directorIP string) (*BOSHManifestConfig, error)`**

Resolve director IP and deployment configuration:
- Auto-select IP from warden network (10.244.0.0/24) if not provided
- Use `SelectAvailableIP()` with preferred range
- Return full config with deployment name format: `bosh-<cpi>`

**`ensureWardenCloudConfig(ui UI) error`**

Apply warden cloud-config if not already present:
- Check if 10.244.0.0/24 network exists in current cloud-config
- If not found, apply warden cloud-config from embedded manifests
- Use `manifests.BOSHWardenCloudConfig()` to get content

**`PrepareBOSHManifestFiles(cpiType string) (*BOSHManifestFiles, error)`**

Create temp directory with all manifest files:
- Write main bosh.yml manifest
- Write all ops files in correct order
- Return paths for interpolation

**`interpolateBOSHManifest(files *BOSHManifestFiles, config *BOSHManifestConfig) ([]byte, error)`**

Run `bosh interpolate` with ops files and variables:
- Variables: `internal_ip`, `garden_host=127.0.0.1`
- Apply all ops files in order
- Return interpolated manifest bytes

**`deployBOSHDirector(manifestPath, deploymentName string) error`**

Execute bosh deploy:
- Run `bosh deploy <manifest> -d <deployment-name> -n`
- Stream output to stdout/stderr for user visibility

**`BOSHDeleteAction(ui UI, opts BOSHDeleteOptions) error`**

Delete BOSH director deployment:
- Construct deployment name from CPI type
- Run `bosh delete-deployment -d bosh-<cpi>`
- Optional `-n` flag for force deletion

#### Helper Functions

**`printBOSHTargetInstructions(ui UI, config *BOSHManifestConfig)`**

Print instructions for targeting the nested director:
- Show how to get CA cert from config-server
- Show how to set BOSH_CLIENT_SECRET from config-server
- Show development workflow example

**`boshDeploymentExists(deploymentName string) (bool, error)`**

Check if deployment already exists:
- Run `bosh deployments --json`
- Parse JSON and check for deployment name

**`hasWardenNetwork(cloudConfigJSON []byte) bool`**

Check if cloud-config contains warden network:
- Simple string check for "10.244.0.0/24" or "10.244.0.34"

---

## Phase 4: CLI Integration

### 4.1 Update `cmd/ibosh/main.go`

Add after the `cf` command group (around line 773):

```go
{
    Name:  "bosh",
    Usage: "Deploy and manage BOSH director",
    Description: `Deploy and manage a BOSH director on instant-bosh.

This deploys a BOSH director as a BOSH deployment (via 'bosh deploy', not 'bosh create-env').
Designed for BOSH director development - enables rapid iteration with dev releases.

The deployed director uses bosh-lite (warden CPI) with Garden RunC running in the VM.
All credentials are stored in the instant-bosh config-server at /instant-bosh/bosh-<cpi>/.

Requires:
  - Incus backend (warden needs full VMs, not Docker containers)
  - BOSH environment configured: eval "$(ibosh incus print-env)"
  - Warden stemcell uploaded: ibosh incus upload-stemcell ubuntu-noble latest

Examples:
  ibosh bosh deploy                          # Deploy bosh-lite director
  ibosh bosh deploy --director-ip 10.244.0.50 # Use specific IP
  ibosh bosh delete                          # Delete deployment`,
    Subcommands: []*cli.Command{
        {
            Name:  "deploy",
            Usage: "Deploy BOSH director",
            Flags: []cli.Flag{
                &cli.StringFlag{
                    Name:  "cpi",
                    Usage: "CPI type (currently only 'warden' is supported)",
                    Value: "warden",
                },
                &cli.StringFlag{
                    Name:  "director-ip",
                    Usage: "Static IP for director (auto-selected from cloud-config if not specified)",
                },
                &cli.BoolFlag{
                    Name:  "dry-run",
                    Usage: "Show what would be deployed without deploying",
                },
            },
            Action: func(c *cli.Context) error {
                ui, _ := initUIAndLogger(c)
                opts := commands.BOSHDeployOptions{
                    CPI:        c.String("cpi"),
                    DirectorIP: c.String("director-ip"),
                    DryRun:     c.Bool("dry-run"),
                }
                return commands.BOSHDeployAction(ui, opts)
            },
        },
        {
            Name:  "delete",
            Usage: "Delete BOSH director deployment",
            Flags: []cli.Flag{
                &cli.StringFlag{
                    Name:  "cpi",
                    Usage: "CPI type (defaults to 'warden')",
                    Value: "warden",
                },
                &cli.BoolFlag{
                    Name:    "force",
                    Aliases: []string{"f"},
                    Usage:   "Skip confirmation prompt",
                },
            },
            Action: func(c *cli.Context) error {
                ui, _ := initUIAndLogger(c)
                opts := commands.BOSHDeleteOptions{
                    CPI:   c.String("cpi"),
                    Force: c.Bool("force"),
                }
                return commands.BOSHDeleteAction(ui, opts)
            },
        },
    },
},
```

---

## Phase 5: Documentation

### 5.1 Update `README.md`

Add new section after the CF deployment section:

```markdown
### Deploying a BOSH Director (Development)

For BOSH director development, `ibosh bosh deploy` deploys a nested BOSH director as a BOSH deployment. This enables rapid iteration with dev releases using `bosh deploy` instead of `bosh create-env`.

**Use Cases:**
- Developing BOSH director features
- Testing BOSH releases in isolation
- Quick iteration on director code changes

**Prerequisites:**
```bash
# 1. Start instant-bosh with Incus (warden requires full VMs)
ibosh incus start
eval "$(ibosh incus print-env)"

# 2. Upload warden stemcell
ibosh incus upload-stemcell ubuntu-noble latest
```

**Deploy BOSH Director:**
```bash
# Deploy bosh-lite director (auto-selects IP from 10.244.0.0/24)
ibosh bosh deploy

# Or specify IP explicitly
ibosh bosh deploy --director-ip 10.244.0.50

# Verify deployment
bosh -d bosh-warden instances
```

**Target the Nested Director:**
```bash
# Get CA cert and credentials from config-server
bosh alias-env bosh-dev -e 10.244.0.34 --ca-cert <(bosh int <(credhub get -n /instant-bosh/bosh-warden/director_ssl -k ca) --path /ca)

export BOSH_ENVIRONMENT=bosh-dev
export BOSH_CLIENT=admin
export BOSH_CLIENT_SECRET=$(credhub get -n /instant-bosh/bosh-warden/admin_password -q)

# Verify
bosh env
```

**Development Workflow:**
```bash
# 1. Make changes to BOSH director code
cd ~/workspace/bosh-release
vim src/bosh-director/...

# 2. Create and upload dev release to nested director
bosh create-release --force
bosh upload-release

# 3. Update the nested director with new release
ibosh bosh deploy

# 4. Test your changes
bosh env
bosh vms
# ... your tests ...

# 5. Iterate quickly - repeat steps 1-4
```

**Cleanup:**
```bash
ibosh bosh delete --force
```

**Notes:**
- The deployment name is `bosh-warden` (format: `bosh-<cpi>`)
- Credentials are stored in config-server at `/instant-bosh/bosh-warden/`
- Warden cloud-config (10.244.0.0/24) is automatically applied
- Only warden CPI is currently supported (Docker/Incus CPI support planned)
```

---

## Error Handling & Edge Cases

### Common Error Scenarios

**1. Wrong Backend (Docker instead of Incus):**
```
Error: warden CPI requires full VMs and is not supported with Docker backend.

Please use Incus backend instead:
  ibosh incus start
  eval "$(ibosh incus print-env)"
  ibosh incus upload-stemcell ubuntu-noble latest
```

**2. BOSH Environment Not Set:**
```
Error: BOSH environment not configured. Missing: BOSH_ENVIRONMENT, BOSH_CLIENT

Run: eval "$(ibosh incus print-env)"
```

**3. Unsupported CPI:**
```
Error: unsupported CPI type: docker (only 'warden' is currently supported)
```

**4. No Available IPs:**
```
Error: no available IPs in static range (all 67 IPs are in use)

Please free up IPs or expand the static range in cloud-config.
```

**5. Cloud-Config Missing:**
- Automatically applied with user notification
- No error, just informational message

---

## Variables and Configuration

### BOSH Variables (Interpolation)

**Required Variables:**
- `internal_ip` - Director IP (from config or auto-selected)
- `garden_host` - Garden address (always `127.0.0.1` for warden)

**Generated Variables (by BOSH):**
All other variables are generated by BOSH and stored in config-server:
- `admin_password`
- `director_ssl` (CA, certificate, private_key)
- `blobstore_director_password`
- `blobstore_agent_password`
- `registry_host_password`
- `postgres_password`
- `nats_password`
- And many more...

**Config-Server Paths:**
Format: `/instant-bosh/<deployment-name>/<variable-name>`

Examples:
- `/instant-bosh/bosh-warden/admin_password`
- `/instant-bosh/bosh-warden/director_ssl`
- `/instant-bosh/bosh-warden/nats_password`

### Warden Cloud-Config

```yaml
azs:
- name: z1

vm_types:
- name: default

disk_types:
- name: default
  disk_size: 1024

networks:
- name: default
  type: manual
  subnets:
  - azs: [z1]
    dns: [8.8.8.8]
    range: 10.244.0.0/24
    gateway: 10.244.0.1
    static: [10.244.0.34-10.244.0.100]  # 67 available IPs
    reserved: []

compilation:
  workers: 5
  az: z1
  reuse_compilation_vms: true
  vm_type: default
  network: default
```

---

## Implementation Checklist

### Phase 1: Foundation
- [ ] Update `vendir.yml` with new bosh-deployment includes
- [ ] Run `vendir sync` to fetch files
- [ ] Create `internal/commands/ipselection.go` with reusable IP helper
- [ ] Refactor `internal/commands/cf.go` to use new IP helper
- [ ] Create `internal/manifests/bosh_manifests.go` with manifest helpers
- [ ] Test IP selection refactoring with CF deployment

### Phase 2: Core Implementation
- [ ] Create `internal/commands/bosh.go` skeleton with all function signatures
- [ ] Implement `BOSHDeployAction()` with full flow
- [ ] Implement `ensureWardenCloudConfig()` for auto-apply
- [ ] Implement `ResolveBOSHConfig()` with IP selection
- [ ] Implement `PrepareBOSHManifestFiles()`
- [ ] Implement `interpolateBOSHManifest()`
- [ ] Implement `deployBOSHDirector()`
- [ ] Implement `BOSHDeleteAction()`
- [ ] Implement helper functions

### Phase 3: CLI Integration
- [ ] Update `cmd/ibosh/main.go` with `bosh` command group
- [ ] Add `bosh deploy` subcommand with flags
- [ ] Add `bosh delete` subcommand with flags
- [ ] Test CLI parsing and option handling

### Phase 4: Manual Testing
- [ ] Test: Deploy bosh-warden on Incus
- [ ] Test: Auto-select IP from 10.244.0.0/24 range
- [ ] Test: Explicit IP with `--director-ip`
- [ ] Test: Verify deployment with `bosh -d bosh-warden instances`
- [ ] Test: Target nested director with config-server credentials
- [ ] Test: Upload dev release to nested director
- [ ] Test: Update deployment with `ibosh bosh deploy`
- [ ] Test: Delete deployment with `ibosh bosh delete`
- [ ] Test: Dry-run mode
- [ ] Test: Error handling (Docker backend, no BOSH env, etc.)
- [ ] Test: Multiple deployments (different IPs)
- [ ] Test: Cloud-config auto-apply

### Phase 5: Documentation
- [ ] Update `README.md` with BOSH deployment section
- [ ] Add prerequisites section
- [ ] Add deployment examples
- [ ] Add development workflow example
- [ ] Document credential paths
- [ ] Add troubleshooting notes
- [ ] Add cleanup instructions

---

## File Structure Summary

### New Files (3)
1. **`internal/commands/ipselection.go`** (~200 lines)
   - Reusable IP selection helper
   - Extract static IPs from cloud-config
   - Filter by network and CIDR range
   - Check used IPs and find available

2. **`internal/commands/bosh.go`** (~450 lines)
   - Main BOSH deployment logic
   - Deployment and deletion actions
   - Configuration resolution
   - Manifest preparation and interpolation
   - Helper functions

3. **`internal/manifests/bosh_manifests.go`** (~80 lines)
   - BOSH manifest access functions
   - Ops file retrieval
   - Cloud-config access

### Modified Files (3)
1. **`internal/commands/cf.go`** (~20 lines changed)
   - Refactor to use IPSelectionOptions
   - Remove inline IP selection logic

2. **`cmd/ibosh/main.go`** (+90 lines)
   - Add `bosh` command group
   - Add `bosh deploy` and `bosh delete` subcommands

3. **`vendir.yml`** (+6 lines)
   - Add bosh-lite ops files
   - Add warden cloud-config
   - Add UAA, CredHub, bosh-dev ops files

### Documentation (1)
1. **`README.md`** (+120 lines)
   - Add BOSH deployment section
   - Prerequisites, deployment, targeting
   - Development workflow

### Total Impact
- **New code:** ~730 lines
- **Modified code:** ~116 lines
- **Documentation:** ~120 lines
- **Total:** ~966 lines

---

## Success Criteria

- [x] `ibosh bosh deploy` successfully deploys a bosh-lite director
- [ ] Deployed director is accessible and functional via IP
- [ ] Can target nested director with config-server credentials
- [ ] Can deploy workloads to the nested director
- [ ] Can upload dev releases and update director via `ibosh bosh deploy`
- [ ] Clean error messages for common failure scenarios
- [ ] Documentation is clear and complete
- [ ] IP selection helper is reusable between CF and BOSH

---

## Future Enhancements

### Docker CPI Support
- Add `--cpi docker` option
- Use `docker/cpi.yml` ops file instead of warden
- Deploy containers instead of VMs
- Deployment name: `bosh-docker`

### Incus CPI Support
- Add `--cpi incus` option
- Create or use incus/lxd CPI ops file
- Deploy VMs using Incus
- Deployment name: `bosh-incus`

### Multiple Deployment Support
- Allow custom deployment names via `--name` flag
- Support multiple nested directors simultaneously
- Example: `ibosh bosh deploy --name bosh-dev-1 --cpi warden`

### Advanced Configuration
- Support custom ops files via `--ops-file` flag
- Support custom variables via `-v` flag
- Integration with existing vars-store files (optional)

### Testing
- Add unit tests for all functions
- Add integration tests
- Add CI pipeline for automated testing

---

## Notes

### Why Warden First?

1. **Full BOSH Features:** Warden provides full VM semantics (unlike Docker containers)
2. **Bosh-Lite Standard:** Warden is the standard bosh-lite CPI
3. **Garden Integration:** Includes Garden RunC out of the box
4. **Development Standard:** Most BOSH developers use bosh-lite

### Why Not Docker First?

1. **Limited Semantics:** Docker containers lack full VM features
2. **Systemd Issues:** Docker containers have systemd limitations
3. **Not Development Standard:** Less commonly used for BOSH director dev

### Why Deployment Name `bosh-<cpi>`?

1. **Multi-CPI Support:** Allows multiple directors with different CPIs
2. **Clear Identification:** Easy to identify which CPI is being used
3. **Future-Proof:** Natural extension point for additional CPIs
4. **Namespace Separation:** Each CPI gets its own config-server namespace

---

## References

- [BOSH Deployment Repository](https://github.com/cloudfoundry/bosh-deployment)
- [Bosh-Lite Documentation](https://bosh.io/docs/bosh-lite.html)
- [Warden CPI Release](https://github.com/cloudfoundry/bosh-warden-cpi-release)
- [BOSH CLI v2 Documentation](https://bosh.io/docs/cli-v2.html)
- [BOSH Config Server](https://bosh.io/docs/config-server.html)
