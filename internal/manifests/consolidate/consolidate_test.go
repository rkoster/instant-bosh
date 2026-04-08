package consolidate_test

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/rkoster/instant-bosh/internal/manifests"
	"github.com/rkoster/instant-bosh/internal/manifests/consolidate"
)

// stubRouterIP is the static IP used in test fixtures.
const stubRouterIP = "10.0.1.5"

// stubSystemDomain is the system domain used in test fixtures.
const stubSystemDomain = "system.example.com"

// buildInterpolatedManifest produces a minimal BOSH manifest that looks like the
// real post-interpolation output, but without running the bosh CLI. It:
//  1. Parses cf-deployment.yml
//  2. Replaces ((router_static_ip)) / ((haproxy_private_ip)) with stubRouterIP
//  3. Replaces ((system_domain)) with stubSystemDomain
//  4. Applies scale-to-one-az (sets instances=1 on the expected groups)
//  5. Adds the haproxy instance group (as use-haproxy.yml would) so we can test removal
//  6. Injects a router static_ip network entry to simulate use-gorouter-static-ip.yml
//
// This is NOT a full bosh interpolate; it is purpose-built for testing consolidation logic.
func buildInterpolatedManifest(t *testing.T) []byte {
	t.Helper()

	rawManifest, err := manifests.CFDeploymentManifest()
	if err != nil {
		t.Fatalf("CFDeploymentManifest() error = %v", err)
	}

	// Replace variable placeholders with stub values.
	content := string(rawManifest)
	content = strings.ReplaceAll(content, "((system_domain))", stubSystemDomain)
	content = strings.ReplaceAll(content, "((router_static_ip))", stubRouterIP)
	content = strings.ReplaceAll(content, "((haproxy_private_ip))", stubRouterIP)

	// Parse into a generic manifest structure.
	var m map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &m); err != nil {
		t.Fatalf("failed to unmarshal manifest: %v", err)
	}

	// Apply scale-to-one-az: set all multi-instance groups to instances=1.
	igs := instanceGroups(t, m)
	for _, ig := range igs {
		igMap := ig.(map[string]interface{})
		igMap["instances"] = 1
		// Normalise AZs to z1 only.
		igMap["azs"] = []interface{}{"z1"}
	}

	// Inject router static IP into the router instance group's network (simulating
	// the use-gorouter-static-ip.yml ops file effect).
	for _, ig := range igs {
		igMap := ig.(map[string]interface{})
		if igMap["name"] == "router" {
			nets, ok := igMap["networks"].([]interface{})
			if ok && len(nets) > 0 {
				net := nets[0].(map[string]interface{})
				net["static_ips"] = []interface{}{stubRouterIP}
			}
		}
	}

	// Append a synthetic haproxy instance group (as use-haproxy.yml adds it).
	haproxyIG := map[string]interface{}{
		"name":      "haproxy",
		"azs":       []interface{}{"z1"},
		"instances": 1,
		"vm_type":   "minimal",
		"stemcell":  "default",
		"networks": []interface{}{
			map[string]interface{}{
				"name":       "default",
				"static_ips": []interface{}{stubRouterIP},
			},
		},
		"jobs": []interface{}{
			map[string]interface{}{
				"name":    "haproxy",
				"release": "haproxy",
			},
		},
	}
	igsList := m["instance_groups"].([]interface{})
	m["instance_groups"] = append(igsList, haproxyIG)

	// Also add haproxy release and variables to test filtering.
	releases := m["releases"].([]interface{})
	m["releases"] = append(releases, map[string]interface{}{
		"name":    "haproxy",
		"version": "16.4.0+3.2.13",
		"url":     "https://bosh.io/d/github.com/cloudfoundry-incubator/haproxy-boshrelease",
		"sha1":    "sha256:c38446fe",
	})

	variables, _ := m["variables"].([]interface{})
	m["variables"] = append(variables,
		map[string]interface{}{"name": "haproxy_ca", "type": "certificate"},
		map[string]interface{}{"name": "haproxy_ssl", "type": "certificate"},
	)

	out, err := yaml.Marshal(m)
	if err != nil {
		t.Fatalf("failed to marshal test manifest: %v", err)
	}
	return out
}

// instanceGroups extracts the instance_groups slice from a generic manifest map.
func instanceGroups(t *testing.T, m map[string]interface{}) []interface{} {
	t.Helper()
	igs, ok := m["instance_groups"].([]interface{})
	if !ok {
		t.Fatal("manifest missing instance_groups")
	}
	return igs
}

// findInstanceGroup finds a specific instance group by name in a consolidated manifest.
func findInstanceGroup(t *testing.T, manifestBytes []byte, name string) *consolidate.InstanceGroup {
	t.Helper()
	var m struct {
		InstanceGroups []consolidate.InstanceGroup `yaml:"instance_groups"`
	}
	if err := yaml.Unmarshal(manifestBytes, &m); err != nil {
		t.Fatalf("failed to parse consolidated manifest: %v", err)
	}
	for i := range m.InstanceGroups {
		if m.InstanceGroups[i].Name == name {
			return &m.InstanceGroups[i]
		}
	}
	return nil
}

// jobNames returns the list of job names in an instance group.
func jobNames(ig *consolidate.InstanceGroup) []string {
	var names []string
	for _, j := range ig.Jobs {
		names = append(names, j.Name)
	}
	return names
}

// containsJob returns true if the job name is present in the instance group.
func containsJob(ig *consolidate.InstanceGroup, name string) bool {
	for _, j := range ig.Jobs {
		if j.Name == name {
			return true
		}
	}
	return false
}

// TestConsolidateAllJobsMapped verifies that no unmapped instance group error is
// returned when running consolidation against the real cf-deployment manifest
// plus the haproxy instance group.
func TestConsolidateAllJobsMapped(t *testing.T) {
	input := buildInterpolatedManifest(t)
	_, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() unexpected error: %v", err)
	}
}

// TestOutputHasFiveGroups checks that the consolidated manifest has exactly the
// five expected instance group names.
func TestOutputHasFiveGroups(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}

	var m struct {
		InstanceGroups []struct {
			Name string `yaml:"name"`
		} `yaml:"instance_groups"`
	}
	if err := yaml.Unmarshal(out, &m); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	expected := []string{"database", "blobstore", "control", "compute", "router"}
	if len(m.InstanceGroups) != len(expected) {
		var got []string
		for _, ig := range m.InstanceGroups {
			got = append(got, ig.Name)
		}
		t.Fatalf("expected %d instance groups %v, got %d: %v", len(expected), expected, len(m.InstanceGroups), got)
	}
	for i, eg := range expected {
		if m.InstanceGroups[i].Name != eg {
			t.Errorf("instance_groups[%d]: expected %q, got %q", i, eg, m.InstanceGroups[i].Name)
		}
	}
}

// TestHaproxyGroupRemoved asserts there is no haproxy instance group in the output.
func TestHaproxyGroupRemoved(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	ig := findInstanceGroup(t, out, "haproxy")
	if ig != nil {
		t.Error("haproxy instance group must be removed but was found in output")
	}
}

// TestHaproxyReleaseRemoved asserts the haproxy release entry is not in the output.
func TestHaproxyReleaseRemoved(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	if strings.Contains(string(out), "name: haproxy") {
		t.Error("haproxy release/instance-group reference must be removed from output")
	}
}

// TestHaproxyVariablesRemoved asserts haproxy_ca and haproxy_ssl variables are removed.
func TestHaproxyVariablesRemoved(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	content := string(out)
	for _, v := range []string{"haproxy_ca", "haproxy_ssl"} {
		if strings.Contains(content, v) {
			t.Errorf("variable %q must be removed but was found in output", v)
		}
	}
}

// TestRouterGroupContainsGorouter asserts the gorouter job is in the router group.
func TestRouterGroupContainsGorouter(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	ig := findInstanceGroup(t, out, "router")
	if ig == nil {
		t.Fatal("router instance group not found")
	}
	if !containsJob(ig, "gorouter") {
		t.Errorf("router group must contain gorouter job, got: %v", jobNames(ig))
	}
}

// TestRouterGroupContainsTCPRouter asserts tcp_router job is in the router group.
func TestRouterGroupContainsTCPRouter(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	ig := findInstanceGroup(t, out, "router")
	if ig == nil {
		t.Fatal("router instance group not found")
	}
	if !containsJob(ig, "tcp_router") {
		t.Errorf("router group must contain tcp_router job, got: %v", jobNames(ig))
	}
}

// TestRouterGroupContainsSSHProxy asserts ssh_proxy is extracted from scheduler
// and placed in the router group.
func TestRouterGroupContainsSSHProxy(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	ig := findInstanceGroup(t, out, "router")
	if ig == nil {
		t.Fatal("router instance group not found")
	}
	if !containsJob(ig, "ssh_proxy") {
		t.Errorf("router group must contain ssh_proxy job, got: %v", jobNames(ig))
	}
}

// TestControlGroupDoesNotContainSSHProxy asserts ssh_proxy is NOT in the control group.
func TestControlGroupDoesNotContainSSHProxy(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	ig := findInstanceGroup(t, out, "control")
	if ig == nil {
		t.Fatal("control instance group not found")
	}
	if containsJob(ig, "ssh_proxy") {
		t.Error("control group must NOT contain ssh_proxy (it belongs in router)")
	}
}

// TestComputeGroupContainsFileServer asserts file_server is extracted from the api
// source IG and placed in the compute group (avoids port 8080/8443 conflict with
// log-cache and UAA which also land in control).
func TestComputeGroupContainsFileServer(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	ig := findInstanceGroup(t, out, "compute")
	if ig == nil {
		t.Fatal("compute instance group not found")
	}
	if !containsJob(ig, "file_server") {
		t.Errorf("compute group must contain file_server job, got: %v", jobNames(ig))
	}
}

// TestControlGroupDoesNotContainFileServer asserts file_server is NOT in control.
func TestControlGroupDoesNotContainFileServer(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	ig := findInstanceGroup(t, out, "control")
	if ig == nil {
		t.Fatal("control instance group not found")
	}
	if containsJob(ig, "file_server") {
		t.Error("control group must NOT contain file_server (it belongs in compute)")
	}
}

// TestRouterGroupHasStaticIP asserts the router group gets the stub static IP.
func TestRouterGroupHasStaticIP(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	ig := findInstanceGroup(t, out, "router")
	if ig == nil {
		t.Fatal("router instance group not found")
	}
	if len(ig.Networks) == 0 {
		t.Fatal("router group has no networks")
	}
	found := false
	for _, net := range ig.Networks {
		for _, ip := range net.StaticIPs {
			if ip == stubRouterIP {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("router group must have static IP %q in networks, got: %+v", stubRouterIP, ig.Networks)
	}
}

// TestDatabaseGroupContainsPXCMySQL asserts database has the pxc-mysql job.
func TestDatabaseGroupContainsPXCMySQL(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	ig := findInstanceGroup(t, out, "database")
	if ig == nil {
		t.Fatal("database instance group not found")
	}
	if !containsJob(ig, "pxc-mysql") {
		t.Errorf("database group must contain pxc-mysql, got: %v", jobNames(ig))
	}
}

// TestControlGroupContainsCredhub asserts the credhub job is in the control group.
// CredHub was moved from database to control because credhub's start script calls
// wait_for_uaa which requires UAA (also in control) to be up before starting.
func TestControlGroupContainsCredhub(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	ig := findInstanceGroup(t, out, "control")
	if ig == nil {
		t.Fatal("control instance group not found")
	}
	if !containsJob(ig, "credhub") {
		t.Errorf("control group must contain credhub, got: %v", jobNames(ig))
	}
	// Also assert credhub is NOT in database.
	dbIG := findInstanceGroup(t, out, "database")
	if dbIG != nil && containsJob(dbIG, "credhub") {
		t.Error("credhub must not be in the database group (it depends on UAA)")
	}
}

// TestDatabaseGroupHasPersistentDisk asserts the database group preserves disk config.
func TestDatabaseGroupHasPersistentDisk(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	ig := findInstanceGroup(t, out, "database")
	if ig == nil {
		t.Fatal("database instance group not found")
	}
	if ig.PersistentDiskType == "" {
		t.Error("database group must have a persistent_disk_type (from source 'database' group)")
	}
}

// TestComputeGroupContainsDiegoCellJobs asserts the compute group has key diego-cell jobs.
func TestComputeGroupContainsDiegoCellJobs(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	ig := findInstanceGroup(t, out, "compute")
	if ig == nil {
		t.Fatal("compute instance group not found")
	}
	for _, required := range []string{"garden", "rep", "silk-daemon"} {
		if !containsJob(ig, required) {
			t.Errorf("compute group must contain %q, got: %v", required, jobNames(ig))
		}
	}
}

// TestBlobstoreGroupContainsBlobstore asserts the blobstore group has the blobstore job.
func TestBlobstoreGroupContainsBlobstore(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	ig := findInstanceGroup(t, out, "blobstore")
	if ig == nil {
		t.Fatal("blobstore instance group not found")
	}
	if !containsJob(ig, "blobstore") {
		t.Errorf("blobstore group must contain blobstore job, got: %v", jobNames(ig))
	}
}

// TestDatabaseGroupContainsNATS asserts the nats-tls job is in the database group.
// nats is placed in database (not control) to avoid a pid_utils package name collision
// between the nats release and the diego release.
func TestDatabaseGroupContainsNATS(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	ig := findInstanceGroup(t, out, "database")
	if ig == nil {
		t.Fatal("database instance group not found")
	}
	if !containsJob(ig, "nats-tls") {
		t.Errorf("database group must contain nats-tls (nats release), got: %v", jobNames(ig))
	}
}

// TestControlGroupDoesNotContainNATS asserts nats-tls is NOT in the control group.
func TestControlGroupDoesNotContainNATS(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	ig := findInstanceGroup(t, out, "control")
	if ig == nil {
		t.Fatal("control instance group not found")
	}
	if containsJob(ig, "nats-tls") {
		t.Error("control group must NOT contain nats-tls (it belongs in database to avoid pid_utils collision)")
	}
}

// TestRouterGroupContainsSmokeTests asserts the smoke_tests job is in the router group.
// smoke-tests is placed in router (not control) to avoid a golang-1-linux package name
// collision between the cf-smoke-tests release and the capi release.
func TestRouterGroupContainsSmokeTests(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	ig := findInstanceGroup(t, out, "router")
	if ig == nil {
		t.Fatal("router instance group not found")
	}
	if !containsJob(ig, "smoke_tests") {
		t.Errorf("router group must contain smoke_tests (cf-smoke-tests release), got: %v", jobNames(ig))
	}
}

// TestControlGroupDoesNotContainSmokeTests asserts smoke_tests is NOT in the control group.
func TestControlGroupDoesNotContainSmokeTests(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	ig := findInstanceGroup(t, out, "control")
	if ig == nil {
		t.Fatal("control instance group not found")
	}
	if containsJob(ig, "smoke_tests") {
		t.Error("control group must NOT contain smoke_tests (it belongs in router to avoid golang-1-linux collision)")
	}
}

// TestControlGroupContainsRotateCCDatabaseKey asserts rotate_cc_database_key is in control.
func TestControlGroupContainsRotateCCDatabaseKey(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	ig := findInstanceGroup(t, out, "control")
	if ig == nil {
		t.Fatal("control instance group not found")
	}
	if !containsJob(ig, "rotate_cc_database_key") {
		t.Errorf("control group must contain rotate_cc_database_key, got: %v", jobNames(ig))
	}
}

// TestControlGroupVMTypeMedium asserts control group uses vm_type: medium.
func TestControlGroupVMTypeMedium(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	ig := findInstanceGroup(t, out, "control")
	if ig == nil {
		t.Fatal("control instance group not found")
	}
	if ig.VMType != "medium" {
		t.Errorf("control group vm_type = %q, want %q", ig.VMType, "medium")
	}
}

// TestRouterGroupVMTypeMinimal asserts router group uses vm_type: minimal.
func TestRouterGroupVMTypeMinimal(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	ig := findInstanceGroup(t, out, "router")
	if ig == nil {
		t.Fatal("router instance group not found")
	}
	if ig.VMType != "minimal" {
		t.Errorf("router group vm_type = %q, want %q", ig.VMType, "minimal")
	}
}

// TestAllGroupsHaveInstancesOne asserts every consolidated group has instances: 1.
func TestAllGroupsHaveInstancesOne(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	var m struct {
		InstanceGroups []consolidate.InstanceGroup `yaml:"instance_groups"`
	}
	if err := yaml.Unmarshal(out, &m); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	for _, ig := range m.InstanceGroups {
		if ig.Instances != 1 {
			t.Errorf("instance group %q: instances = %d, want 1", ig.Name, ig.Instances)
		}
	}
}

// TestNoduplicateJobsPerGroup asserts that each consolidated instance group contains
// no duplicate job names. This catches cases where multiple source groups contribute
// the same job (e.g. cfdot appears in diego-api, scheduler, and diego-cell).
func TestNoDuplicateJobsPerGroup(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}
	var m struct {
		InstanceGroups []consolidate.InstanceGroup `yaml:"instance_groups"`
	}
	if err := yaml.Unmarshal(out, &m); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	for _, ig := range m.InstanceGroups {
		seen := map[string]bool{}
		for _, job := range ig.Jobs {
			if seen[job.Name] {
				t.Errorf("instance group %q has duplicate job %q", ig.Name, job.Name)
			}
			seen[job.Name] = true
		}
	}
}

// TestUnmappedJobsError asserts that an instance group not in sourceMapping causes an error.
func TestUnmappedJobsError(t *testing.T) {
	// Minimal manifest with one unknown instance group.
	input := []byte(`
name: cf
update:
  canaries: 1
  canary_watch_time: 30000
  max_in_flight: 1
  serial: false
  update_watch_time: 5000
releases:
- name: my-release
  version: "1.0"
stemcells:
- alias: default
  os: ubuntu-jammy
  version: "1.2"
instance_groups:
- name: unknown-group-xyz
  azs: [z1]
  instances: 1
  vm_type: minimal
  stemcell: default
  networks:
  - name: default
  jobs:
  - name: some-job
    release: my-release
`)
	_, err := consolidate.ConsolidateInstanceGroups(input)
	if err == nil {
		t.Fatal("expected an error for unmapped instance group, got nil")
	}
	if !strings.Contains(err.Error(), "unknown-group-xyz") {
		t.Errorf("error should mention the unknown group name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "unmapped") {
		t.Errorf("error should mention 'unmapped', got: %v", err)
	}
}

// TestAddonAliasesRewritten asserts that the bosh-dns-aliases addon's target instance_group
// values are rewritten from original IG names to consolidated group names.
func TestAddonAliasesRewritten(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}

	// Parse the output to find the bosh-dns-aliases addon.
	var m struct {
		Addons []struct {
			Name string `yaml:"name"`
			Jobs []struct {
				Name       string `yaml:"name"`
				Properties struct {
					Aliases []struct {
						Domain  string `yaml:"domain"`
						Targets []struct {
							InstanceGroup string `yaml:"instance_group"`
						} `yaml:"targets"`
					} `yaml:"aliases"`
				} `yaml:"properties"`
			} `yaml:"jobs"`
		} `yaml:"addons"`
	}
	if err := yaml.Unmarshal(out, &m); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	// Build a set of valid consolidated group names.
	validGroups := map[string]bool{
		"control": true, "compute": true, "database": true,
		"router": true, "blobstore": true,
	}

	// Cases we specifically want to assert.
	wantDomainIG := map[string]string{
		"nats.service.cf.internal":        "database", // nats moved to database
		"sql-db.service.cf.internal":      "database",
		"credhub.service.cf.internal":     "control", // credhub moved to control (depends on UAA)
		"bbs.service.cf.internal":         "control",
		"auctioneer.service.cf.internal":  "control",
		"ssh-proxy.service.cf.internal":   "router", // ssh_proxy extracted from scheduler → router
		"blobstore.service.cf.internal":   "blobstore",
		"doppler.service.cf.internal":     "blobstore", // doppler moved to blobstore (avoids port 8082 conflict)
		"gorouter.service.cf.internal":    "router",
		"file-server.service.cf.internal": "compute", // file_server extracted from api → compute (avoids port 8080/8443 conflict with log-cache/UAA)
	}

	var dnsAliasJob *struct {
		Name       string `yaml:"name"`
		Properties struct {
			Aliases []struct {
				Domain  string `yaml:"domain"`
				Targets []struct {
					InstanceGroup string `yaml:"instance_group"`
				} `yaml:"targets"`
			} `yaml:"aliases"`
		} `yaml:"properties"`
	}
	for i := range m.Addons {
		if m.Addons[i].Name == "bosh-dns-aliases" {
			for j := range m.Addons[i].Jobs {
				if m.Addons[i].Jobs[j].Name == "bosh-dns-aliases" {
					dnsAliasJob = &m.Addons[i].Jobs[j]
					break
				}
			}
		}
	}
	if dnsAliasJob == nil {
		t.Fatal("bosh-dns-aliases addon/job not found in output")
	}

	// Build a map of domain → instance_groups for assertion.
	domainIGs := map[string][]string{}
	for _, alias := range dnsAliasJob.Properties.Aliases {
		for _, tgt := range alias.Targets {
			domainIGs[alias.Domain] = append(domainIGs[alias.Domain], tgt.InstanceGroup)
		}
		// Assert: no alias target points to a pre-consolidation IG name.
		for _, tgt := range alias.Targets {
			ig := tgt.InstanceGroup
			// Pre-consolidation names that must NOT appear (known source IGs
			// that were renamed/merged — does not include "database" or "router"
			// which happen to map to themselves).
			nonConsolidatedSources := []string{
				"nats", "diego-api", "uaa", "api", "cc-worker", "scheduler",
				"log-cache", "doppler", "log-api", "rotate-cc-database-key",
				"credhub", "singleton-blobstore", "tcp-router",
			}
			for _, old := range nonConsolidatedSources {
				if ig == old {
					t.Errorf("alias %q still references pre-consolidation IG %q", alias.Domain, ig)
				}
			}
			// All remaining known targets must be valid consolidated group names
			// OR optional cell types not in our manifest (isolated-diego-cell, windows2019-cell).
			optionalCells := map[string]bool{"isolated-diego-cell": true, "windows2019-cell": true}
			if !validGroups[ig] && !optionalCells[ig] {
				t.Errorf("alias %q has unexpected instance_group %q", alias.Domain, ig)
			}
		}
	}

	// Assert specific domain→IG mappings.
	for domain, wantIG := range wantDomainIG {
		igs, ok := domainIGs[domain]
		if !ok {
			t.Errorf("alias domain %q not found in output", domain)
			continue
		}
		found := false
		for _, ig := range igs {
			if ig == wantIG {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("alias %q: want instance_group %q, got %v", domain, wantIG, igs)
		}
	}
}

// TestInstanceGroupUpdateSerial asserts that each consolidated instance group carries
// the correct update.serial value as declared in the instanceGroups spec:
// database must be serial:true; all others must be serial:false.
func TestInstanceGroupUpdateSerial(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}

	// Expected serial values per group name, derived directly from the instanceGroups spec.
	expectedSerial := map[string]bool{
		"database":  true,
		"blobstore": false,
		"control":   true,
		"compute":   false,
		"router":    false,
	}

	var m struct {
		InstanceGroups []struct {
			Name   string `yaml:"name"`
			Update struct {
				Serial bool `yaml:"serial"`
			} `yaml:"update"`
		} `yaml:"instance_groups"`
	}
	if err := yaml.Unmarshal(out, &m); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	for _, ig := range m.InstanceGroups {
		want, ok := expectedSerial[ig.Name]
		if !ok {
			t.Errorf("unexpected instance group %q in output", ig.Name)
			continue
		}
		if ig.Update.Serial != want {
			t.Errorf("instance group %q: update.serial = %v, want %v", ig.Name, ig.Update.Serial, want)
		}
	}
}

// TestComputeVMExtensions asserts that the compute instance group carries the
// 100GB_ephemeral_disk vm_extension sourced from the diego-cell instance group.
// This extension is required for the rep (Diego executor) to have enough ephemeral
// disk for grootfs overlay loop devices; without it rep fails with
// "auto disk limit must result in a positive number".
func TestComputeVMExtensions(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}

	var m struct {
		InstanceGroups []struct {
			Name         string   `yaml:"name"`
			VMExtensions []string `yaml:"vm_extensions"`
		} `yaml:"instance_groups"`
	}
	if err := yaml.Unmarshal(out, &m); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	var compute *struct {
		Name         string   `yaml:"name"`
		VMExtensions []string `yaml:"vm_extensions"`
	}
	for i := range m.InstanceGroups {
		if m.InstanceGroups[i].Name == "compute" {
			compute = &m.InstanceGroups[i]
			break
		}
	}
	if compute == nil {
		t.Fatal("compute instance group not found in output")
	}

	found := false
	for _, ext := range compute.VMExtensions {
		if ext == "100GB_ephemeral_disk" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("compute instance group missing 100GB_ephemeral_disk vm_extension; got: %v", compute.VMExtensions)
	}
}

// TestRouteRegistrarRoutesMerged asserts that route_registrar routes from all source
// instance groups in a consolidated group are merged into a single route_registrar job.
// In particular, the api IG and uaa IG both have route_registrar jobs that land in
// control; the output must contain a single route_registrar job with all routes.
func TestRouteRegistrarRoutesMerged(t *testing.T) {
	input := buildInterpolatedManifest(t)
	out, err := consolidate.ConsolidateInstanceGroups(input)
	if err != nil {
		t.Fatalf("ConsolidateInstanceGroups() error = %v", err)
	}

	control := findInstanceGroup(t, out, "control")

	// Find the single route_registrar job.
	var rrJob *consolidate.Job
	for i := range control.Jobs {
		if control.Jobs[i].Name == "route_registrar" {
			if rrJob != nil {
				t.Fatal("control group has more than one route_registrar job")
			}
			j := control.Jobs[i]
			rrJob = &j
		}
	}
	if rrJob == nil {
		t.Fatal("control group has no route_registrar job")
	}

	// Extract routes.
	rrProps, _ := rrJob.Properties["route_registrar"].(map[string]interface{})
	if rrProps == nil {
		t.Fatal("route_registrar job has no route_registrar properties")
	}
	routes, _ := rrProps["routes"].([]interface{})

	// Collect route names.
	routeNames := map[string]bool{}
	for _, r := range routes {
		rm, _ := r.(map[string]interface{})
		if rm == nil {
			continue
		}
		name, _ := rm["name"].(string)
		routeNames[name] = true
	}

	// uaa comes from the uaa IG; api/policy-server/routing-api come from the api IG.
	for _, want := range []string{"uaa", "api", "policy-server", "routing-api"} {
		if !routeNames[want] {
			t.Errorf("control route_registrar missing route %q; got routes: %v", want, routeNames)
		}
	}
}
