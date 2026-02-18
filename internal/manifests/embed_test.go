package manifests_test

import (
	"strings"
	"testing"

	"github.com/rkoster/instant-bosh/internal/manifests"
)

func TestCFDeploymentManifest(t *testing.T) {
	manifest, err := manifests.CFDeploymentManifest()
	if err != nil {
		t.Fatalf("CFDeploymentManifest() error = %v", err)
	}

	if len(manifest) == 0 {
		t.Error("CFDeploymentManifest() returned empty content")
	}

	// Check that it's a valid YAML with expected content
	content := string(manifest)
	if !strings.Contains(content, "name: cf") {
		t.Error("CFDeploymentManifest() should contain 'name: cf'")
	}
}

func TestCFDeploymentOpsFile(t *testing.T) {
	opsFile, err := manifests.CFDeploymentOpsFile("scale-to-one-az.yml")
	if err != nil {
		t.Fatalf("CFDeploymentOpsFile() error = %v", err)
	}

	if len(opsFile) == 0 {
		t.Error("CFDeploymentOpsFile() returned empty content")
	}
}

func TestCFDeploymentOpsFileExperimental(t *testing.T) {
	opsFile, err := manifests.CFDeploymentOpsFileExperimental("fast-deploy-with-downtime-and-danger.yml")
	if err != nil {
		t.Fatalf("CFDeploymentOpsFileExperimental() error = %v", err)
	}

	if len(opsFile) == 0 {
		t.Error("CFDeploymentOpsFileExperimental() returned empty content")
	}
}

func TestStandardCFOpsFiles(t *testing.T) {
	opsFiles, err := manifests.StandardCFOpsFiles()
	if err != nil {
		t.Fatalf("StandardCFOpsFiles() error = %v", err)
	}

	expectedCount := 5 // scale-to-one-az, use-compiled-releases, set-router-static-ips, fast-deploy, use-create-swap-delete
	if len(opsFiles) != expectedCount {
		t.Errorf("StandardCFOpsFiles() returned %d ops files, expected %d", len(opsFiles), expectedCount)
	}

	for i, content := range opsFiles {
		if len(content) == 0 {
			t.Errorf("StandardCFOpsFiles()[%d] is empty", i)
		}
	}
}

func TestListCFDeploymentOpsFiles(t *testing.T) {
	files, err := manifests.ListCFDeploymentOpsFiles()
	if err != nil {
		t.Fatalf("ListCFDeploymentOpsFiles() error = %v", err)
	}

	if len(files) == 0 {
		t.Error("ListCFDeploymentOpsFiles() returned empty list")
	}

	// Check that expected files are present
	expectedFiles := []string{
		"scale-to-one-az.yml",
		"use-compiled-releases.yml",
	}

	for _, expected := range expectedFiles {
		found := false
		for _, f := range files {
			if f == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ListCFDeploymentOpsFiles() missing expected file: %s", expected)
		}
	}
}
