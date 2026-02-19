package cpi

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// CPIType represents the type of CPI in use
type CPIType string

const (
	// CPITypeDocker represents the Docker CPI
	CPITypeDocker CPIType = "docker"

	// CPITypeIncus represents the Incus/LXD CPI
	CPITypeIncus CPIType = "incus"

	// CPITypeUnknown represents an unknown CPI type
	CPITypeUnknown CPIType = "unknown"
)

// boshEnvResponse represents the JSON response from `bosh env --json`
type boshEnvResponse struct {
	Tables []struct {
		Rows []struct {
			CPI string `json:"cpi"`
		} `json:"Rows"`
	} `json:"Tables"`
}

// DetectCPIType detects the CPI type from `bosh env --json` output
// Returns CPITypeDocker for "docker_cpi", CPITypeIncus for "lxd_cpi"
func DetectCPIType(ctx context.Context) (CPIType, error) {
	cmd := exec.CommandContext(ctx, "bosh", "env", "--json")
	output, err := cmd.Output()
	if err != nil {
		return CPITypeUnknown, fmt.Errorf("running bosh env: %w", err)
	}

	return ParseCPITypeFromBoshEnv(output)
}

// ParseCPITypeFromBoshEnv parses the CPI type from bosh env JSON output
// This is exported for testing
func ParseCPITypeFromBoshEnv(output []byte) (CPIType, error) {
	var response boshEnvResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return CPITypeUnknown, fmt.Errorf("parsing bosh env output: %w", err)
	}

	if len(response.Tables) == 0 || len(response.Tables[0].Rows) == 0 {
		return CPITypeUnknown, fmt.Errorf("no CPI information in bosh env output")
	}

	cpiName := response.Tables[0].Rows[0].CPI

	switch cpiName {
	case "docker_cpi":
		return CPITypeDocker, nil
	case "lxd_cpi":
		return CPITypeIncus, nil
	default:
		return CPITypeUnknown, fmt.Errorf("unknown CPI type: %s", cpiName)
	}
}

// CPITypeFromInstance returns the CPI type based on the CPI instance type
func CPITypeFromInstance(cpiInstance CPI) CPIType {
	switch cpiInstance.(type) {
	case *DockerCPI:
		return CPITypeDocker
	case *IncusCPI:
		return CPITypeIncus
	default:
		return CPITypeUnknown
	}
}
