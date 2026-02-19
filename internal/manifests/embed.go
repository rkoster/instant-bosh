package manifests

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
)

//go:embed cf-deployment
var cfDeploymentFS embed.FS

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
		{"use-create-swap-delete-vm-strategy.yml", true},
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
