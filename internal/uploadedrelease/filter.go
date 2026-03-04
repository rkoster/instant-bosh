// Package uploadedrelease provides utilities for filtering BOSH manifests
// to skip re-downloading releases that are already uploaded to the director.
package uploadedrelease

import (
	"fmt"

	boshdir "github.com/cloudfoundry/bosh-cli/v7/director"
	boshtpl "github.com/cloudfoundry/bosh-cli/v7/director/template"
	"github.com/cppforlife/go-patch/patch"
	"gopkg.in/yaml.v2"
)

// ReleaseChecker is an interface for checking if releases exist on a BOSH director.
// This matches the relevant subset of boshdir.Director for easier testing.
type ReleaseChecker interface {
	Releases() ([]boshdir.Release, error)
}

// Filter removes url and sha1 fields from releases in the manifest that already
// exist on the BOSH director (matched by name and version only).
// This prevents bosh deploy from re-downloading already uploaded releases.
//
// If the director cannot be reached or returns an error, the original manifest
// is returned unchanged to allow the deployment to proceed.
func Filter(manifestBytes []byte, director ReleaseChecker) ([]byte, error) {
	// Get existing releases from director
	existingReleases, err := director.Releases()
	if err != nil {
		// Return original manifest unchanged if we can't query the director
		return manifestBytes, nil
	}

	// Build lookup map of existing releases: "name:version" -> true
	existingMap := make(map[string]bool)
	for _, rel := range existingReleases {
		key := fmt.Sprintf("%s:%s", rel.Name(), rel.Version().String())
		existingMap[key] = true
	}

	// Parse manifest to extract release names and versions
	var manifest struct {
		Releases []struct {
			Name    string `yaml:"name"`
			Version string `yaml:"version"`
		} `yaml:"releases"`
	}
	if err := yaml.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Build ops to remove url and sha1 for existing releases
	var opDefs []patch.OpDefinition
	for _, rel := range manifest.Releases {
		key := fmt.Sprintf("%s:%s", rel.Name, rel.Version)
		if existingMap[key] {
			// Use optional path (?) so removal doesn't fail if field is missing
			urlPath := fmt.Sprintf("/releases/name=%s/url?", rel.Name)
			sha1Path := fmt.Sprintf("/releases/name=%s/sha1?", rel.Name)
			opDefs = append(opDefs,
				patch.OpDefinition{Type: "remove", Path: &urlPath},
				patch.OpDefinition{Type: "remove", Path: &sha1Path},
			)
		}
	}

	// If no ops to apply, return original manifest
	if len(opDefs) == 0 {
		return manifestBytes, nil
	}

	// Convert op definitions to ops
	ops, err := patch.NewOpsFromDefinitions(opDefs)
	if err != nil {
		return nil, fmt.Errorf("failed to create patch ops: %w", err)
	}

	// Use bosh template to apply ops and get consistent YAML output
	tpl := boshtpl.NewTemplate(manifestBytes)
	result, err := tpl.Evaluate(boshtpl.StaticVariables{}, ops, boshtpl.EvaluateOpts{})
	if err != nil {
		return nil, fmt.Errorf("failed to apply ops to manifest: %w", err)
	}

	return result, nil
}
