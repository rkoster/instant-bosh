package manifests

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed cf-deployment
var cfDeploymentFS embed.FS

//go:embed bosh-deployment
var boshDeploymentFS embed.FS

//go:embed cf-ops
var cfOpsFS embed.FS

// CFDeploymentManifest returns the main cf-deployment.yml manifest
func CFDeploymentManifest() ([]byte, error) {
	return cfDeploymentFS.ReadFile("cf-deployment/cf-deployment.yml")
}

// CFDeploymentOpsFile returns a specific ops file from cf-deployment/operations
func CFDeploymentOpsFile(name string) ([]byte, error) {
	path := filepath.Join("cf-deployment/operations", name)
	return cfDeploymentFS.ReadFile(path)
}

// CFDeploymentOpsFileExperimental returns a specific experimental ops file
func CFDeploymentOpsFileExperimental(name string) ([]byte, error) {
	path := filepath.Join("cf-deployment/operations/experimental", name)
	return cfDeploymentFS.ReadFile(path)
}

// CFOpsFile returns a specific ops file from the cf-ops directory (instant-bosh specific)
func CFOpsFile(name string) ([]byte, error) {
	path := filepath.Join("cf-ops", name)
	return cfOpsFS.ReadFile(path)
}

// ListCFDeploymentOpsFiles lists all available ops files
func ListCFDeploymentOpsFiles() ([]string, error) {
	var files []string
	err := fs.WalkDir(cfDeploymentFS, "cf-deployment/operations", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".yml" {
			// Return relative path from operations directory
			rel, _ := filepath.Rel("cf-deployment/operations", path)
			files = append(files, rel)
		}
		return nil
	})
	return files, err
}

// StandardCFOpsFiles returns the standard ops files for deploying CF to instant-bosh
func StandardCFOpsFiles() ([][]byte, error) {
	opsFiles := []struct {
		path         string
		experimental bool
	}{
		{"scale-to-one-az.yml", false},
		{"use-compiled-releases.yml", false},
		{"use-haproxy.yml", false},
		{"fast-deploy-with-downtime-and-danger.yml", true},
	}

	var result [][]byte
	for _, op := range opsFiles {
		var content []byte
		var err error
		if op.experimental {
			content, err = CFDeploymentOpsFileExperimental(op.path)
		} else {
			content, err = CFDeploymentOpsFile(op.path)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read ops file %s: %w", op.path, err)
		}
		result = append(result, content)
	}

	// Add instant-bosh specific CF ops files
	iboshOpsFiles := []string{
		"skip-rep-drain.yml",
	}
	for _, name := range iboshOpsFiles {
		content, err := CFOpsFile(name)
		if err != nil {
			return nil, fmt.Errorf("failed to read ops file %s: %w", name, err)
		}
		result = append(result, content)
	}

	return result, nil
}

// DNSRuntimeConfig returns the bosh-dns runtime-config from bosh-deployment
func DNSRuntimeConfig() ([]byte, error) {
	return boshDeploymentFS.ReadFile("bosh-deployment/runtime-configs/dns.yml")
}

// DNSOpsFile generates an ops file that injects bosh-dns from the runtime-config
// into a deployment manifest. It transforms the runtime-config addons, releases,
// and variables into BOSH ops file operations that append to the deployment manifest.
func DNSOpsFile() ([]byte, error) {
	dnsConfig, err := DNSRuntimeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to read DNS runtime-config: %w", err)
	}

	// Parse the runtime-config YAML
	var config struct {
		Addons    []interface{} `yaml:"addons"`
		Releases  []interface{} `yaml:"releases"`
		Variables []interface{} `yaml:"variables"`
	}
	if err := yaml.Unmarshal(dnsConfig, &config); err != nil {
		return nil, fmt.Errorf("failed to parse DNS runtime-config: %w", err)
	}

	// Generate ops file operations
	var ops []map[string]interface{}

	// Append each release
	for _, rel := range config.Releases {
		ops = append(ops, map[string]interface{}{
			"type":  "replace",
			"path":  "/releases/-",
			"value": rel,
		})
	}

	// Append each addon
	for _, addon := range config.Addons {
		ops = append(ops, map[string]interface{}{
			"type":  "replace",
			"path":  "/addons/-",
			"value": addon,
		})
	}

	// Append each variable
	for _, variable := range config.Variables {
		ops = append(ops, map[string]interface{}{
			"type":  "replace",
			"path":  "/variables/-",
			"value": variable,
		})
	}

	return yaml.Marshal(ops)
}

// CompiledReleasesStemcellOpsFile generates an ops file that updates all compiled
// releases to use the stemcell version from cf-deployment.yml. This allows
// pre-compiled packages to be used with newer compatible stemcells.
func CompiledReleasesStemcellOpsFile() ([]byte, error) {
	// Read cf-deployment.yml and extract stemcell version
	manifest, err := CFDeploymentManifest()
	if err != nil {
		return nil, fmt.Errorf("failed to read cf-deployment.yml: %w", err)
	}

	// Parse the manifest to get stemcell version
	var manifestData struct {
		Stemcells []struct {
			Alias   string `yaml:"alias"`
			OS      string `yaml:"os"`
			Version string `yaml:"version"`
		} `yaml:"stemcells"`
	}
	if err := yaml.Unmarshal(manifest, &manifestData); err != nil {
		return nil, fmt.Errorf("failed to parse cf-deployment.yml: %w", err)
	}

	if len(manifestData.Stemcells) == 0 {
		return nil, fmt.Errorf("no stemcells found in cf-deployment.yml")
	}

	// Get the stemcell version from the manifest (typically the "default" stemcell)
	stemcellVersion := manifestData.Stemcells[0].Version
	if stemcellVersion == "" {
		return nil, fmt.Errorf("no stemcell version found in cf-deployment.yml")
	}

	// Read use-compiled-releases.yml to get list of releases with stemcell sections
	compiledReleases, err := CFDeploymentOpsFile("use-compiled-releases.yml")
	if err != nil {
		return nil, fmt.Errorf("failed to read use-compiled-releases.yml: %w", err)
	}

	// Parse the ops file YAML to extract release names that have stemcell sections
	// Structure: [{type, path, value: {name, sha1, stemcell: {os, version}, url, version}}]
	var opsEntries []struct {
		Type  string `yaml:"type"`
		Path  string `yaml:"path"`
		Value struct {
			Name     string `yaml:"name"`
			Stemcell struct {
				OS      string `yaml:"os"`
				Version string `yaml:"version"`
			} `yaml:"stemcell"`
		} `yaml:"value"`
	}
	if err := yaml.Unmarshal(compiledReleases, &opsEntries); err != nil {
		return nil, fmt.Errorf("failed to parse use-compiled-releases.yml: %w", err)
	}

	// Generate ops file entries to update each release's stemcell version
	var ops []map[string]interface{}
	for _, entry := range opsEntries {
		// Only process entries that have a stemcell section
		if entry.Value.Name != "" && entry.Value.Stemcell.Version != "" {
			ops = append(ops, map[string]interface{}{
				"type":  "replace",
				"path":  fmt.Sprintf("/releases/name=%s/stemcell/version", entry.Value.Name),
				"value": stemcellVersion,
			})
		}
	}

	if len(ops) == 0 {
		return nil, fmt.Errorf("no compiled releases with stemcell sections found")
	}

	return yaml.Marshal(ops)
}
