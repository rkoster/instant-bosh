package commands_test

import (
	"os"
	"testing"

	"github.com/cloudfoundry/bosh-cli/v7/director/template"
	"github.com/cppforlife/go-patch/patch"
	"github.com/rkoster/instant-bosh/internal/commands"
	"github.com/rkoster/instant-bosh/internal/cpi"
	"github.com/rkoster/instant-bosh/internal/manifests/consolidate"
	yamlv2 "gopkg.in/yaml.v2"
	"gopkg.in/yaml.v3"
)

// Exercise the same embedded manifest and ordered ops files used by both
// cf manifest and cf deploy, including instance group consolidation.
func TestCFManifestDisablesKernelParametersOnlyForDocker(t *testing.T) {
	for _, cpiType := range []cpi.CPIType{cpi.CPITypeDocker, cpi.CPITypeIncus, cpi.CPITypeUnknown} {
		t.Run(string(cpiType), func(t *testing.T) {
			testCFManifestKernelParameters(t, cpiType)
		})
	}
}

func testCFManifestKernelParameters(t *testing.T, cpiType cpi.CPIType) {
	t.Helper()
	files, err := commands.PrepareCFManifestFiles(cpiType)
	if err != nil {
		t.Fatal(err)
	}
	defer files.Cleanup()

	raw, err := os.ReadFile(files.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var definitions []patch.OpDefinition
	for _, path := range files.OpsPaths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var ops []patch.OpDefinition
		// Use BOSH's YAML parser and interpolation engine so this regression
		// runs in the normal Go suite without requiring an external bosh CLI.
		if err := yamlv2.Unmarshal(content, &ops); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		definitions = append(definitions, ops...)
	}
	ops, err := patch.NewOpsFromDefinitions(definitions)
	if err != nil {
		t.Fatal(err)
	}
	interpolated, err := template.NewTemplate(raw).Evaluate(template.StaticVariables{
		"system_domain":    "system.example.com",
		"router_static_ip": "10.0.1.5",
	}, ops, template.EvaluateOpts{})
	if err != nil {
		t.Fatalf("interpolate CF manifest: %v", err)
	}

	consolidated, err := consolidate.ConsolidateInstanceGroups(interpolated)
	if err != nil {
		t.Fatal(err)
	}

	for _, stage := range []struct {
		name     string
		manifest []byte
		groups   map[string]string
	}{
		{"interpolated", interpolated, map[string]string{
			"rep": "diego-cell", "bbs": "diego-api", "file_server": "api", "locket": "diego-api",
		}},
		{"consolidated", consolidated, map[string]string{
			"rep": "compute", "bbs": "control", "file_server": "control", "locket": "control",
		}},
	} {
		t.Run(stage.name, func(t *testing.T) {
			var manifest struct {
				InstanceGroups []struct {
					Name string `yaml:"name"`
					Jobs []struct {
						Name       string         `yaml:"name"`
						Properties map[string]any `yaml:"properties"`
					} `yaml:"jobs"`
				} `yaml:"instance_groups"`
			}
			if err := yaml.Unmarshal(stage.manifest, &manifest); err != nil {
				t.Fatal(err)
			}
			counts := map[string]int{}
			for _, group := range manifest.InstanceGroups {
				for _, job := range group.Jobs {
					wantGroup, targeted := stage.groups[job.Name]
					if !targeted {
						continue
					}
					counts[job.Name]++
					if group.Name != wantGroup {
						t.Errorf("%s is in %q, want %q", job.Name, group.Name, wantGroup)
					}
					if cpiType == cpi.CPITypeDocker {
						value, ok := job.Properties["set_kernel_parameters"].(bool)
						if !ok || value {
							t.Errorf("%s properties.set_kernel_parameters = %#v, want boolean false", job.Name, job.Properties["set_kernel_parameters"])
						}
					} else if value, exists := job.Properties["set_kernel_parameters"]; exists {
						t.Errorf("non-Docker %s overrides set_kernel_parameters with %#v, want release default", job.Name, value)
					}
				}
			}
			for job := range stage.groups {
				if counts[job] != 1 {
					t.Errorf("found %d %s jobs, want 1", counts[job], job)
				}
			}
		})
	}
}
