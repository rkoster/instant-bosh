package boshio_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rkoster/instant-bosh/internal/boshio"
)

func TestResolveStemcell_Latest(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/stemcells/bosh-openstack-kvm-ubuntu-jammy-go_agent" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}

		response := []map[string]interface{}{
			{
				"name":    "bosh-openstack-kvm-ubuntu-jammy-go_agent",
				"version": "1.586",
				"regular": map[string]interface{}{
					"url":  "https://storage.googleapis.com/bosh-core-stemcells/1.586/bosh-stemcell-1.586-openstack-kvm-ubuntu-jammy-go_agent.tgz",
					"sha1": "abc123def456",
					"size": 653127680,
				},
			},
			{
				"name":    "bosh-openstack-kvm-ubuntu-jammy-go_agent",
				"version": "1.585",
				"regular": map[string]interface{}{
					"url":  "https://storage.googleapis.com/bosh-core-stemcells/1.585/bosh-stemcell-1.585-openstack-kvm-ubuntu-jammy-go_agent.tgz",
					"sha1": "def789ghi012",
					"size": 653000000,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := boshio.NewClient(boshio.WithBaseURL(server.URL))

	info, err := client.ResolveStemcell(context.Background(), "bosh-openstack-kvm-ubuntu-jammy-go_agent", "latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Name != "bosh-openstack-kvm-ubuntu-jammy-go_agent" {
		t.Errorf("expected name bosh-openstack-kvm-ubuntu-jammy-go_agent, got %s", info.Name)
	}
	if info.Version != "1.586" {
		t.Errorf("expected version 1.586, got %s", info.Version)
	}
	if info.SHA1 != "abc123def456" {
		t.Errorf("expected sha1 abc123def456, got %s", info.SHA1)
	}
	if info.Size != 653127680 {
		t.Errorf("expected size 653127680, got %d", info.Size)
	}
}

func TestResolveStemcell_SpecificVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := []map[string]interface{}{
			{
				"name":    "bosh-openstack-kvm-ubuntu-jammy-go_agent",
				"version": "1.586",
				"regular": map[string]interface{}{
					"url":  "https://example.com/1.586.tgz",
					"sha1": "sha1-586",
					"size": 100,
				},
			},
			{
				"name":    "bosh-openstack-kvm-ubuntu-jammy-go_agent",
				"version": "1.585",
				"regular": map[string]interface{}{
					"url":  "https://example.com/1.585.tgz",
					"sha1": "sha1-585",
					"size": 200,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := boshio.NewClient(boshio.WithBaseURL(server.URL))

	info, err := client.ResolveStemcell(context.Background(), "bosh-openstack-kvm-ubuntu-jammy-go_agent", "1.585")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Version != "1.585" {
		t.Errorf("expected version 1.585, got %s", info.Version)
	}
	if info.SHA1 != "sha1-585" {
		t.Errorf("expected sha1 sha1-585, got %s", info.SHA1)
	}
}

func TestResolveStemcell_VersionNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := []map[string]interface{}{
			{
				"name":    "bosh-openstack-kvm-ubuntu-jammy-go_agent",
				"version": "1.586",
				"regular": map[string]interface{}{
					"url":  "https://example.com/1.586.tgz",
					"sha1": "sha1-586",
					"size": 100,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := boshio.NewClient(boshio.WithBaseURL(server.URL))

	_, err := client.ResolveStemcell(context.Background(), "bosh-openstack-kvm-ubuntu-jammy-go_agent", "1.500")
	if err == nil {
		t.Fatal("expected error for non-existent version")
	}
}

func TestResolveStemcell_StemcellNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := boshio.NewClient(boshio.WithBaseURL(server.URL))

	_, err := client.ResolveStemcell(context.Background(), "nonexistent-stemcell", "latest")
	if err == nil {
		t.Fatal("expected error for non-existent stemcell")
	}
}

func TestOpenStackStemcellName(t *testing.T) {
	tests := []struct {
		os       string
		expected string
	}{
		{"ubuntu-jammy", "bosh-openstack-kvm-ubuntu-jammy-go_agent"},
		{"ubuntu-noble", "bosh-openstack-kvm-ubuntu-noble-go_agent"},
		{"ubuntu-bionic", "bosh-openstack-kvm-ubuntu-bionic-go_agent"},
	}

	for _, tt := range tests {
		t.Run(tt.os, func(t *testing.T) {
			result := boshio.OpenStackStemcellName(tt.os)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestIncusStemcellName(t *testing.T) {
	// IncusStemcellName should return the same as OpenStackStemcellName
	result := boshio.IncusStemcellName("ubuntu-jammy")
	expected := "bosh-openstack-kvm-ubuntu-jammy-go_agent"
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestResolveOpenStackStemcell_Noble(t *testing.T) {
	// Test that Noble stemcells are resolved without -go_agent suffix
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Noble stemcells don't have -go_agent suffix
		if r.URL.Path == "/stemcells/bosh-openstack-kvm-ubuntu-noble" {
			response := []map[string]interface{}{
				{
					"name":    "bosh-openstack-kvm-ubuntu-noble",
					"version": "1.268",
					"regular": map[string]interface{}{
						"url":  "https://storage.googleapis.com/bosh-core-stemcells/1.268/bosh-stemcell-1.268-openstack-kvm-ubuntu-noble.tgz",
						"sha1": "noble-sha1",
						"size": 1310303402,
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}

		// Return empty array for -go_agent suffix version (simulating bosh.io behavior)
		if r.URL.Path == "/stemcells/bosh-openstack-kvm-ubuntu-noble-go_agent" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{})
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	client := boshio.NewClient(boshio.WithBaseURL(server.URL))

	info, err := client.ResolveOpenStackStemcell(context.Background(), "ubuntu-noble", "latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Name != "bosh-openstack-kvm-ubuntu-noble" {
		t.Errorf("expected name bosh-openstack-kvm-ubuntu-noble, got %s", info.Name)
	}
	if info.Version != "1.268" {
		t.Errorf("expected version 1.268, got %s", info.Version)
	}
}

func TestResolveOpenStackStemcell_Jammy_Fallback(t *testing.T) {
	// Test that Jammy stemcells fall back to -go_agent suffix
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Jammy without suffix returns empty (not found)
		if r.URL.Path == "/stemcells/bosh-openstack-kvm-ubuntu-jammy" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{})
			return
		}

		// Jammy with -go_agent suffix works
		if r.URL.Path == "/stemcells/bosh-openstack-kvm-ubuntu-jammy-go_agent" {
			response := []map[string]interface{}{
				{
					"name":    "bosh-openstack-kvm-ubuntu-jammy-go_agent",
					"version": "1.586",
					"regular": map[string]interface{}{
						"url":  "https://storage.googleapis.com/bosh-core-stemcells/1.586/bosh-stemcell-1.586-openstack-kvm-ubuntu-jammy-go_agent.tgz",
						"sha1": "jammy-sha1",
						"size": 653127680,
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	client := boshio.NewClient(boshio.WithBaseURL(server.URL))

	info, err := client.ResolveOpenStackStemcell(context.Background(), "ubuntu-jammy", "latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Name != "bosh-openstack-kvm-ubuntu-jammy-go_agent" {
		t.Errorf("expected name bosh-openstack-kvm-ubuntu-jammy-go_agent, got %s", info.Name)
	}
	if info.Version != "1.586" {
		t.Errorf("expected version 1.586, got %s", info.Version)
	}
}

func TestResolveOpenStackStemcell_NotFound(t *testing.T) {
	// Test that both attempts fail properly
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return empty for both attempts
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{})
	}))
	defer server.Close()

	client := boshio.NewClient(boshio.WithBaseURL(server.URL))

	_, err := client.ResolveOpenStackStemcell(context.Background(), "ubuntu-nonexistent", "latest")
	if err == nil {
		t.Fatal("expected error for non-existent stemcell")
	}
}
