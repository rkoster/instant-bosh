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
