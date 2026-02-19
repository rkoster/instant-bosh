package stemcell_test

import (
	"testing"

	"github.com/rkoster/instant-bosh/internal/stemcell"
)

func TestParseFromManifest_WithOS(t *testing.T) {
	manifest := []byte(`
name: cf
stemcells:
- alias: default
  os: ubuntu-jammy
  version: "1.586"
`)

	reqs, err := stemcell.ParseFromManifest(manifest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reqs) != 1 {
		t.Fatalf("expected 1 requirement, got %d", len(reqs))
	}

	if reqs[0].Alias != "default" {
		t.Errorf("expected alias 'default', got %q", reqs[0].Alias)
	}
	if reqs[0].OS != "ubuntu-jammy" {
		t.Errorf("expected OS 'ubuntu-jammy', got %q", reqs[0].OS)
	}
	if reqs[0].Version != "1.586" {
		t.Errorf("expected version '1.586', got %q", reqs[0].Version)
	}
}

func TestParseFromManifest_WithName(t *testing.T) {
	manifest := []byte(`
name: zookeeper
stemcells:
- alias: default
  name: bosh-openstack-kvm-ubuntu-jammy-go_agent
  version: latest
`)

	reqs, err := stemcell.ParseFromManifest(manifest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reqs) != 1 {
		t.Fatalf("expected 1 requirement, got %d", len(reqs))
	}

	if reqs[0].OS != "ubuntu-jammy" {
		t.Errorf("expected OS 'ubuntu-jammy', got %q", reqs[0].OS)
	}
	if reqs[0].Version != "latest" {
		t.Errorf("expected version 'latest', got %q", reqs[0].Version)
	}
}

func TestParseFromManifest_MultipleStemcells(t *testing.T) {
	manifest := []byte(`
name: cf
stemcells:
- alias: default
  os: ubuntu-jammy
  version: "1.586"
- alias: windows
  os: windows2019
  version: "2019.65"
`)

	reqs, err := stemcell.ParseFromManifest(manifest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reqs) != 2 {
		t.Fatalf("expected 2 requirements, got %d", len(reqs))
	}

	if reqs[0].OS != "ubuntu-jammy" {
		t.Errorf("expected first OS 'ubuntu-jammy', got %q", reqs[0].OS)
	}
	if reqs[1].OS != "windows2019" {
		t.Errorf("expected second OS 'windows2019', got %q", reqs[1].OS)
	}
}

func TestParseFromManifest_NoStemcells(t *testing.T) {
	manifest := []byte(`
name: cf
releases:
- name: cf
  version: latest
`)

	_, err := stemcell.ParseFromManifest(manifest)
	if err == nil {
		t.Fatal("expected error for manifest without stemcells")
	}
}

func TestParseFromManifest_InvalidYAML(t *testing.T) {
	manifest := []byte(`
this is not valid yaml: [
`)

	_, err := stemcell.ParseFromManifest(manifest)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestExtractOSFromStemcellName(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		wantErr  bool
	}{
		{"bosh-openstack-kvm-ubuntu-jammy-go_agent", "ubuntu-jammy", false},
		{"bosh-warden-boshlite-ubuntu-jammy-go_agent", "ubuntu-jammy", false},
		{"bosh-docker-ubuntu-noble", "ubuntu-noble", false},
		{"bosh-aws-xen-hvm-ubuntu-bionic-go_agent", "ubuntu-bionic", false},
		{"invalid-stemcell-name", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := stemcell.ExtractOSFromStemcellName(tt.name)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %q", tt.name)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error for %q: %v", tt.name, err)
				return
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
