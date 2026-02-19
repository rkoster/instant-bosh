package stemcell

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Requirement represents a stemcell required by a BOSH manifest
type Requirement struct {
	Alias   string // Stemcell alias (e.g., "default")
	OS      string // OS name (e.g., "ubuntu-jammy")
	Version string // Version (e.g., "1.586" or "latest")
}

// manifestStemcell represents a stemcell entry in a BOSH manifest
type manifestStemcell struct {
	Alias   string `yaml:"alias"`
	OS      string `yaml:"os"`
	Version string `yaml:"version"`
	Name    string `yaml:"name"` // Alternative to os, full stemcell name
}

// manifest represents the structure of a BOSH manifest for stemcell parsing
type manifest struct {
	Stemcells []manifestStemcell `yaml:"stemcells"`
}

// ParseFromManifest extracts stemcell requirements from a BOSH manifest
// manifest should be the YAML content (e.g., output of `bosh interpolate`)
func ParseFromManifest(manifestYAML []byte) ([]Requirement, error) {
	var m manifest
	if err := yaml.Unmarshal(manifestYAML, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest YAML: %w", err)
	}

	if len(m.Stemcells) == 0 {
		return nil, fmt.Errorf("no stemcells found in manifest")
	}

	var requirements []Requirement
	for _, s := range m.Stemcells {
		req := Requirement{
			Alias:   s.Alias,
			Version: s.Version,
		}

		// Handle both "os" and "name" fields
		if s.OS != "" {
			req.OS = s.OS
		} else if s.Name != "" {
			// Extract OS from full stemcell name
			// e.g., "bosh-openstack-kvm-ubuntu-jammy-go_agent" -> "ubuntu-jammy"
			os, err := ExtractOSFromStemcellName(s.Name)
			if err != nil {
				return nil, fmt.Errorf("parsing stemcell name %q: %w", s.Name, err)
			}
			req.OS = os
		} else {
			return nil, fmt.Errorf("stemcell %q has neither 'os' nor 'name' field", s.Alias)
		}

		requirements = append(requirements, req)
	}

	return requirements, nil
}

// ExtractOSFromStemcellName extracts the OS name from a full stemcell name
// e.g., "bosh-openstack-kvm-ubuntu-jammy-go_agent" -> "ubuntu-jammy"
// e.g., "bosh-warden-boshlite-ubuntu-jammy-go_agent" -> "ubuntu-jammy"
// e.g., "bosh-docker-ubuntu-noble" -> "ubuntu-noble"
func ExtractOSFromStemcellName(name string) (string, error) {
	// Common patterns:
	// bosh-{iaas}-{hypervisor}-{os}-go_agent (e.g., bosh-openstack-kvm-ubuntu-jammy-go_agent)
	// bosh-warden-boshlite-{os}-go_agent (e.g., bosh-warden-boshlite-ubuntu-jammy-go_agent)
	// bosh-docker-{os} (e.g., bosh-docker-ubuntu-noble)

	// Try to find "ubuntu-" pattern and extract from there
	patterns := []string{"ubuntu-"}
	for _, pattern := range patterns {
		idx := -1
		for i := 0; i <= len(name)-len(pattern); i++ {
			if name[i:i+len(pattern)] == pattern {
				idx = i
				break
			}
		}

		if idx >= 0 {
			// Extract from "ubuntu-" to either "-go_agent" or end of string
			remaining := name[idx:]
			// Remove "-go_agent" suffix if present
			suffix := "-go_agent"
			if len(remaining) > len(suffix) && remaining[len(remaining)-len(suffix):] == suffix {
				remaining = remaining[:len(remaining)-len(suffix)]
			}
			return remaining, nil
		}
	}

	return "", fmt.Errorf("cannot extract OS from stemcell name: %s", name)
}
