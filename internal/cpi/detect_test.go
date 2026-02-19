package cpi_test

import (
	"testing"

	"github.com/rkoster/instant-bosh/internal/cpi"
)

func TestParseCPITypeFromBoshEnv_Docker(t *testing.T) {
	output := []byte(`{
		"Tables": [
			{
				"Rows": [
					{
						"cpi": "docker_cpi",
						"name": "instant-bosh",
						"uuid": "12345",
						"version": "280.0.17"
					}
				]
			}
		]
	}`)

	cpiType, err := cpi.ParseCPITypeFromBoshEnv(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cpiType != cpi.CPITypeDocker {
		t.Errorf("expected CPITypeDocker, got %v", cpiType)
	}
}

func TestParseCPITypeFromBoshEnv_Incus(t *testing.T) {
	output := []byte(`{
		"Tables": [
			{
				"Rows": [
					{
						"cpi": "lxd_cpi",
						"name": "instant-bosh",
						"uuid": "12345",
						"version": "280.0.17"
					}
				]
			}
		]
	}`)

	cpiType, err := cpi.ParseCPITypeFromBoshEnv(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cpiType != cpi.CPITypeIncus {
		t.Errorf("expected CPITypeIncus, got %v", cpiType)
	}
}

func TestParseCPITypeFromBoshEnv_Unknown(t *testing.T) {
	output := []byte(`{
		"Tables": [
			{
				"Rows": [
					{
						"cpi": "aws_cpi",
						"name": "my-bosh",
						"uuid": "12345",
						"version": "280.0.17"
					}
				]
			}
		]
	}`)

	_, err := cpi.ParseCPITypeFromBoshEnv(output)
	if err == nil {
		t.Fatal("expected error for unknown CPI type")
	}
}

func TestParseCPITypeFromBoshEnv_EmptyTables(t *testing.T) {
	output := []byte(`{"Tables": []}`)

	_, err := cpi.ParseCPITypeFromBoshEnv(output)
	if err == nil {
		t.Fatal("expected error for empty tables")
	}
}

func TestParseCPITypeFromBoshEnv_InvalidJSON(t *testing.T) {
	output := []byte(`not valid json`)

	_, err := cpi.ParseCPITypeFromBoshEnv(output)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
