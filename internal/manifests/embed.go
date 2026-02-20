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
		{"set-router-static-ips.yml", false},
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
