package consolidate

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// targetGroup is the name of a consolidated instance group.
type targetGroup string

const (
	groupControl   targetGroup = "control"
	groupCompute   targetGroup = "compute"
	groupDatabase  targetGroup = "database"
	groupRouter    targetGroup = "router"
	groupBlobstore targetGroup = "blobstore"
)

// sourceMapping maps each original instance group name to its target consolidated group.
// Any instance group that is removed entirely (e.g. haproxy) maps to the empty string "".
// A source not present in this map causes ConsolidateInstanceGroups to return an error.
var sourceMapping = map[string]targetGroup{
	// control plane
	// NOTE: "nats" is in database (not here) to avoid pid_utils package collision
	// between nats release and diego release.
	// NOTE: "smoke-tests" is in router (not here) to avoid golang-1-linux package
	// collision between cf-smoke-tests release and capi release.
	// NOTE: "credhub" is in control (not database) because credhub's start script
	// (wait_for_uaa) requires UAA to be up first; UAA is also in control.
	// NOTE: "doppler" is in blobstore (not control) because both doppler and
	// reverse_log_proxy (from log-api) default to port 8082; keeping them on
	// separate VMs avoids the bind conflict.
	// NOTE: file_server (in the api IG) binds 0.0.0.0:8443 by default, which
	// conflicts with UAA. This is resolved via an ops file patch
	// (fix-colocation-port-conflicts.yml) that moves file_server to 0.0.0.0:8447.
	"diego-api":              groupControl,
	"uaa":                    groupControl,
	"api":                    groupControl,
	"cc-worker":              groupControl,
	"scheduler":              groupControl, // ssh_proxy job is extracted separately into router
	"log-cache":              groupControl,
	"log-api":                groupControl,
	"rotate-cc-database-key": groupControl,
	"credhub":                groupControl,

	// compute
	"diego-cell": groupCompute,

	// database (+ nats: avoids pid_utils collision with diego in control)
	// NOTE: credhub is in control (not database) because its start script calls
	// wait_for_uaa, which requires UAA to be up. UAA is also in control, so both
	// start together on the same VM after database is fully deployed.
	"nats":     groupDatabase,
	"database": groupDatabase,

	// router (gorouter + tcp-router; ssh_proxy extracted from scheduler;
	// smoke-tests errand colocated here to avoid golang-1-linux collision with capi)
	"smoke-tests": groupRouter,
	"router":      groupRouter,
	"tcp-router":  groupRouter,

	// blobstore (+ doppler: avoids port 8082 collision with reverse_log_proxy in control)
	"singleton-blobstore": groupBlobstore,
	"doppler":             groupBlobstore,

	// removed entirely
	"haproxy": "",
}

// jobsExtractedFromScheduler lists job names that must be moved out of the scheduler
// source instance group and placed directly into the router target group.
var jobsExtractedFromScheduler = map[string]bool{
	"ssh_proxy": true,
}

// groupSpec defines the properties of a consolidated instance group.
type groupSpec struct {
	name   targetGroup
	vmType string
	serial bool // value for the per-IG update.serial field
}

// instanceGroups defines all consolidated instance groups in deployment order.
// A group with serial:true acts as a BOSH "fence": all preceding parallel groups
// must finish before it deploys alone, after which the next batch starts in parallel.
//
// Deployment order:
//  1. database  – serial:true  – PXC MySQL, NATS deploy alone first; all groups depend on MySQL.
//  2. blobstore – serial:false – singleton-blobstore + doppler; no dependency on control.
//     doppler is here (not control) to avoid port 8082 conflict with reverse_log_proxy.
//  3. control   – serial:true  – UAA, CredHub, Diego API, CC, loggregator deploy alone;
//     gorouter needs UAA, silk-daemon needs silk-controller (both in control),
//     so compute and router must not start until control is fully up.
//  4. compute   – serial:false – Diego cells start in parallel with router once control is up.
//  5. router    – serial:false – gorouter, tcp_router, ssh_proxy start in parallel with compute.
//
// NOTE: credhub is colocated with UAA in control (not database) because credhub's
// start script (wait_for_uaa) blocks until UAA is reachable.
var instanceGroups = []groupSpec{
	{groupDatabase, "small", true},
	{groupBlobstore, "small", false},
	{groupControl, "medium", true},
	{groupCompute, "small-highmem", false},
	{groupRouter, "minimal", false},
}

// manifestDoc is the top-level structure of a BOSH deployment manifest.
// We use yaml.Node throughout so we can round-trip arbitrary YAML without losing
// structure that we don't explicitly model.
type manifestDoc struct {
	raw *yaml.Node
}

// InstanceGroup is a lightweight typed view used during consolidation.
type InstanceGroup struct {
	Name               string                 `yaml:"name"`
	Lifecycle          string                 `yaml:"lifecycle,omitempty"`
	AZs                []string               `yaml:"azs"`
	Instances          int                    `yaml:"instances"`
	VMType             string                 `yaml:"vm_type"`
	VMExtensions       []string               `yaml:"vm_extensions,omitempty"`
	Stemcell           string                 `yaml:"stemcell"`
	PersistentDiskType string                 `yaml:"persistent_disk_type,omitempty"`
	Update             map[string]interface{} `yaml:"update,omitempty"`
	Networks           []Network              `yaml:"networks"`
	Jobs               []Job                  `yaml:"jobs"`
	Env                map[string]interface{} `yaml:"env,omitempty"`
}

// Network is a network attachment entry.
type Network struct {
	Name      string   `yaml:"name"`
	StaticIPs []string `yaml:"static_ips,omitempty"`
}

// Job is a job entry inside an instance group.
type Job struct {
	Name       string                 `yaml:"name"`
	Release    string                 `yaml:"release"`
	Provides   map[string]interface{} `yaml:"provides,omitempty"`
	Consumes   map[string]interface{} `yaml:"consumes,omitempty"`
	Properties map[string]interface{} `yaml:"properties,omitempty"`
}

// Manifest is the parts of the BOSH manifest we need to rewrite.
type Manifest struct {
	Name            string                 `yaml:"name"`
	ManifestVersion string                 `yaml:"manifest_version,omitempty"`
	Update          map[string]interface{} `yaml:"update"`
	Addons          []interface{}          `yaml:"addons,omitempty"`
	Releases        []interface{}          `yaml:"releases"`
	Stemcells       []interface{}          `yaml:"stemcells"`
	InstanceGroups  []InstanceGroup        `yaml:"instance_groups"`
	Variables       []interface{}          `yaml:"variables,omitempty"`
	Tags            map[string]interface{} `yaml:"tags,omitempty"`
	Features        map[string]interface{} `yaml:"features,omitempty"`
}

// ConsolidateInstanceGroups takes a fully-interpolated BOSH manifest (YAML bytes)
// and returns a new manifest where instance groups are consolidated into the five
// target groups: control, compute, database, router, blobstore.
//
// The haproxy instance group is removed entirely.
// The ssh_proxy job is extracted from scheduler and placed in router.
//
// An error is returned if any instance group in the manifest is not present in the
// known source mapping — this acts as a safety net to ensure new upstream instance
// groups do not silently disappear from the deployment.
func ConsolidateInstanceGroups(manifestBytes []byte) ([]byte, error) {
	var m Manifest
	if err := yaml.Unmarshal(manifestBytes, &m); err != nil {
		return nil, fmt.Errorf("consolidate: failed to parse manifest: %w", err)
	}

	// Validate: every source instance group must be in the mapping.
	if err := validateMapping(m.InstanceGroups); err != nil {
		return nil, err
	}

	// Build consolidated groups.
	consolidated, err := buildConsolidated(m.InstanceGroups)
	if err != nil {
		return nil, err
	}

	// Filter haproxy variables from the variables section.
	m.Variables = filterHaproxyVariables(m.Variables)

	// Filter haproxy releases from the releases section.
	m.Releases = filterHaproxyReleases(m.Releases)

	// Rewrite bosh-dns-aliases addon targets to use consolidated instance group names.
	m.Addons = rewriteAddonAliases(m.Addons)

	// Replace instance groups with consolidated ones.
	m.InstanceGroups = consolidated

	out, err := yaml.Marshal(&m)
	if err != nil {
		return nil, fmt.Errorf("consolidate: failed to marshal manifest: %w", err)
	}
	return out, nil
}

// validateMapping checks that every instance group in the manifest has an entry in sourceMapping.
func validateMapping(groups []InstanceGroup) error {
	var unknown []string
	for _, ig := range groups {
		if _, ok := sourceMapping[ig.Name]; !ok {
			unknown = append(unknown, ig.Name)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf(
			"consolidate: unmapped instance groups: [%s] — add them to sourceMapping in consolidate.go",
			strings.Join(unknown, ", "),
		)
	}
	return nil
}

// consolidated is the working state during the merge pass.
type consolidated struct {
	jobs              []Job
	seenJobs          map[string]int // tracks job names already added; value is index into jobs slice
	hasPersistentDisk bool
	persistentDisk    string
	staticIPs         []string
	vmExtensions      []string        // union of vm_extensions from all source IGs
	seenVMExtensions  map[string]bool // deduplication for vm_extensions
}

// addJob appends job to the consolidated state. For most jobs, duplicates by name
// are silently dropped. route_registrar is special: when a second route_registrar is
// encountered its routes are merged into the first one, so that all source IGs
// contribute their routes to a single route_registrar job.
func (c *consolidated) addJob(job Job) {
	if c.seenJobs == nil {
		c.seenJobs = map[string]int{}
	}
	idx, exists := c.seenJobs[job.Name]
	if !exists {
		c.seenJobs[job.Name] = len(c.jobs)
		c.jobs = append(c.jobs, job)
		return
	}
	// Special handling: merge route_registrar routes from duplicate jobs.
	if job.Name == "route_registrar" {
		mergeRouteRegistrarRoutes(&c.jobs[idx], job)
	}
	// All other duplicates are dropped (first one wins).
}

// mergeRouteRegistrarRoutes appends the routes from src into dst's
// route_registrar.routes property list.
func mergeRouteRegistrarRoutes(dst *Job, src Job) {
	// Ensure properties maps exist.
	if dst.Properties == nil {
		dst.Properties = map[string]interface{}{}
	}
	srcProps, _ := src.Properties["route_registrar"].(map[string]interface{})
	if srcProps == nil {
		return
	}
	srcRoutes, _ := srcProps["routes"].([]interface{})
	if len(srcRoutes) == 0 {
		return
	}

	dstRR, _ := dst.Properties["route_registrar"].(map[string]interface{})
	if dstRR == nil {
		dstRR = map[string]interface{}{}
		dst.Properties["route_registrar"] = dstRR
	}
	dstRoutes, _ := dstRR["routes"].([]interface{})
	dstRR["routes"] = append(dstRoutes, srcRoutes...)
}

// addVMExtension appends ext to the consolidated vm_extensions unless already present.
func (c *consolidated) addVMExtension(ext string) {
	if c.seenVMExtensions == nil {
		c.seenVMExtensions = map[string]bool{}
	}
	if c.seenVMExtensions[ext] {
		return
	}
	c.seenVMExtensions[ext] = true
	c.vmExtensions = append(c.vmExtensions, ext)
}

// buildConsolidated merges source instance groups into the target groups and returns
// the ordered list of resulting InstanceGroup values.
//
// Errand source instance groups (lifecycle: errand) have their jobs merged into the
// corresponding long-running consolidated group just like any other source IG. The
// consolidated VMs are always running, so errand jobs become persistent co-located
// processes rather than one-off errand runs. The source IG's lifecycle field is not
// carried over.
func buildConsolidated(sources []InstanceGroup) ([]InstanceGroup, error) {
	// Working state per target group.
	state := map[targetGroup]*consolidated{}
	for _, spec := range instanceGroups {
		state[spec.name] = &consolidated{}
	}

	for _, src := range sources {
		tg := sourceMapping[src.Name]
		if tg == "" {
			// Explicitly removed (e.g. haproxy).
			continue
		}

		s := state[tg]

		// Collect persistent disk info for database/blobstore groups.
		if src.PersistentDiskType != "" {
			s.hasPersistentDisk = true
			// Use the first disk type we encounter (they should all be compatible
			// within a group, or the ops file author needs to reconcile).
			if s.persistentDisk == "" {
				s.persistentDisk = src.PersistentDiskType
			}
		}

		// Collect vm_extensions (e.g. 100GB_ephemeral_disk on diego-cell).
		for _, ext := range src.VMExtensions {
			s.addVMExtension(ext)
		}

		// Collect static IPs from network configs (for router).
		for _, net := range src.Networks {
			if len(net.StaticIPs) > 0 {
				s.staticIPs = append(s.staticIPs, net.StaticIPs...)
			}
		}

		// Handle scheduler specially: extract ssh_proxy to router.
		if src.Name == "scheduler" {
			routerState := state[groupRouter]
			for _, job := range src.Jobs {
				if jobsExtractedFromScheduler[job.Name] {
					routerState.addJob(job)
				} else {
					s.addJob(job)
				}
			}
			continue
		}

		// All other source IGs: merge jobs into the consolidated group.
		// This includes errand source IGs (smoke-tests, rotate-cc-database-key):
		// their jobs are merged into the long-running consolidated VM so they run
		// as persistent co-located processes, not one-off errand runs.
		for _, job := range src.Jobs {
			s.addJob(job)
		}
	}

	// Build the final InstanceGroup slice in declaration order.
	var result []InstanceGroup
	for _, spec := range instanceGroups {
		s := state[spec.name]
		if len(s.jobs) == 0 {
			continue
		}

		ig := InstanceGroup{
			Name:      string(spec.name),
			AZs:       []string{"z1"},
			Instances: 1,
			VMType:    spec.vmType,
			Stemcell:  "default",
			Update:    map[string]interface{}{"serial": spec.serial},
			Networks: []Network{
				{Name: "default"},
			},
		}

		if s.hasPersistentDisk {
			ig.PersistentDiskType = s.persistentDisk
		}

		if len(s.vmExtensions) > 0 {
			ig.VMExtensions = s.vmExtensions
		}

		// Assign static IPs to router's network config.
		if spec.name == groupRouter && len(s.staticIPs) > 0 {
			// Deduplicate.
			seen := map[string]bool{}
			var unique []string
			for _, ip := range s.staticIPs {
				if !seen[ip] {
					seen[ip] = true
					unique = append(unique, ip)
				}
			}
			ig.Networks[0].StaticIPs = unique
		}

		ig.Jobs = s.jobs

		result = append(result, ig)
	}

	return result, nil
}

// filterHaproxyVariables removes haproxy-related variable entries from the variables list.
func filterHaproxyVariables(vars []interface{}) []interface{} {
	haproxyVarPrefixes := []string{"haproxy_ca", "haproxy_ssl"}
	var filtered []interface{}
	for _, v := range vars {
		m, ok := v.(map[string]interface{})
		if !ok {
			filtered = append(filtered, v)
			continue
		}
		name, _ := m["name"].(string)
		skip := false
		for _, prefix := range haproxyVarPrefixes {
			if name == prefix {
				skip = true
				break
			}
		}
		if !skip {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

// filterHaproxyReleases removes the haproxy release entry from the releases list.
func filterHaproxyReleases(releases []interface{}) []interface{} {
	var filtered []interface{}
	for _, r := range releases {
		m, ok := r.(map[string]interface{})
		if !ok {
			filtered = append(filtered, r)
			continue
		}
		if name, _ := m["name"].(string); name == "haproxy" {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

// domainOverrides maps specific DNS alias domains to a consolidated group name,
// overriding the default sourceMapping lookup. This is needed for cases where a
// job was extracted from its source IG into a different consolidated group (e.g.
// ssh_proxy is extracted from scheduler→router, but scheduler itself maps to control).
var domainOverrides = map[string]targetGroup{
	"ssh-proxy.service.cf.internal": groupRouter,
}

// rewriteAddonAliases rewrites the bosh-dns-aliases addon so that every alias
// target's instance_group field is replaced with the consolidated group name.
// Targets that reference instance groups mapped to "" (removed groups, e.g. haproxy)
// are dropped from the alias's target list. Alias entries whose target list becomes
// empty after filtering are also dropped.
//
// domainOverrides takes precedence over sourceMapping for specific domains where
// a job was extracted into a different consolidated group than its source IG maps to.
func rewriteAddonAliases(addons []interface{}) []interface{} {
	for _, addon := range addons {
		addonMap, ok := addon.(map[string]interface{})
		if !ok {
			continue
		}
		if addonMap["name"] != "bosh-dns-aliases" {
			continue
		}
		jobs, ok := addonMap["jobs"].([]interface{})
		if !ok {
			continue
		}
		for _, job := range jobs {
			jobMap, ok := job.(map[string]interface{})
			if !ok {
				continue
			}
			props, ok := jobMap["properties"].(map[string]interface{})
			if !ok {
				continue
			}
			aliases, ok := props["aliases"].([]interface{})
			if !ok {
				continue
			}
			var rewrittenAliases []interface{}
			for _, alias := range aliases {
				aliasMap, ok := alias.(map[string]interface{})
				if !ok {
					rewrittenAliases = append(rewrittenAliases, alias)
					continue
				}
				targets, ok := aliasMap["targets"].([]interface{})
				if !ok {
					rewrittenAliases = append(rewrittenAliases, alias)
					continue
				}
				domain, _ := aliasMap["domain"].(string)
				var rewrittenTargets []interface{}
				for _, target := range targets {
					targetMap, ok := target.(map[string]interface{})
					if !ok {
						rewrittenTargets = append(rewrittenTargets, target)
						continue
					}
					ig, _ := targetMap["instance_group"].(string)
					// Domain-level overrides take precedence (e.g. ssh_proxy lives in
					// router even though its source IG "scheduler" maps to control).
					if override, exists := domainOverrides[domain]; exists {
						targetMap["instance_group"] = string(override)
					} else if mapped, exists := sourceMapping[ig]; exists {
						if mapped == "" {
							// Drop targets for removed groups (e.g. haproxy).
							continue
						}
						targetMap["instance_group"] = string(mapped)
					}
					rewrittenTargets = append(rewrittenTargets, targetMap)
				}
				// Drop aliases whose target list became empty.
				if len(rewrittenTargets) == 0 {
					continue
				}
				aliasMap["targets"] = rewrittenTargets
				rewrittenAliases = append(rewrittenAliases, aliasMap)
			}
			props["aliases"] = rewrittenAliases
		}
	}
	return addons
}
