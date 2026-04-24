package commands

import (
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

// IPSelector provides methods for selecting available IPs from BOSH cloud-config
type IPSelector struct {
	ui UI
}

// NewIPSelector creates a new IP selector
func NewIPSelector(ui UI) *IPSelector {
	return &IPSelector{ui: ui}
}

// SelectAvailableIP selects an available IP from the cloud-config static range for the given network
// If networkName is empty, it uses "default"
func (s *IPSelector) SelectAvailableIP(networkName string) (string, error) {
	if networkName == "" {
		networkName = "default"
	}

	// Get cloud-config from BOSH
	cmd := exec.Command("bosh", "cloud-config", "--json")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get cloud-config: %w", err)
	}

	// Parse JSON output - BOSH CLI uses "Blocks" for cloud-config output
	var result struct {
		Tables []struct {
			Rows []struct {
				Content string `json:"content"`
			} `json:"Rows"`
		} `json:"Tables"`
		Blocks []string `json:"Blocks"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return "", fmt.Errorf("failed to parse cloud-config output: %w", err)
	}

	// Try Blocks first (newer BOSH CLI format), then Tables (older format)
	var cloudConfigYAML string
	if len(result.Blocks) > 0 {
		cloudConfigYAML = result.Blocks[0]
	} else if len(result.Tables) > 0 && len(result.Tables[0].Rows) > 0 {
		cloudConfigYAML = result.Tables[0].Rows[0].Content
	} else {
		return "", fmt.Errorf("no cloud-config found")
	}

	// Parse cloud-config YAML
	var cloudConfig struct {
		Networks []struct {
			Name    string `yaml:"name"`
			Subnets []struct {
				Static []string `yaml:"static"`
			} `yaml:"subnets"`
		} `yaml:"networks"`
	}
	if err := yaml.Unmarshal([]byte(cloudConfigYAML), &cloudConfig); err != nil {
		return "", fmt.Errorf("failed to parse cloud-config YAML: %w", err)
	}

	// Find static IPs from specified network
	var staticRange string
	for _, network := range cloudConfig.Networks {
		if network.Name == networkName && len(network.Subnets) > 0 && len(network.Subnets[0].Static) > 0 {
			staticRange = network.Subnets[0].Static[0]
			break
		}
	}

	if staticRange == "" {
		return "", fmt.Errorf("no static IP range found in cloud-config for network '%s'", networkName)
	}

	// Parse static range (format: "10.245.0.34-10.245.0.100")
	availableIPs, err := parseIPRange(staticRange)
	if err != nil {
		return "", fmt.Errorf("failed to parse static IP range: %w", err)
	}

	if len(availableIPs) == 0 {
		return "", fmt.Errorf("no IPs in static range")
	}

	// Get currently used IPs from BOSH instances
	usedIPs, err := getUsedIPs()
	if err != nil {
		s.ui.PrintLinef("Warning: could not get used IPs, may conflict: %v", err)
	}

	// Find first available IP
	for _, ip := range availableIPs {
		if !usedIPs[ip] {
			return ip, nil
		}
	}

	return "", fmt.Errorf("no available IPs in static range (all %d IPs are in use)", len(availableIPs))
}

// parseIPRange parses an IP range string like "10.245.0.34-10.245.0.100"
func parseIPRange(rangeStr string) ([]string, error) {
	parts := strings.Split(rangeStr, "-")
	if len(parts) != 2 {
		// Single IP
		return []string{rangeStr}, nil
	}

	startIP := net.ParseIP(strings.TrimSpace(parts[0]))
	endIP := net.ParseIP(strings.TrimSpace(parts[1]))

	if startIP == nil || endIP == nil {
		return nil, fmt.Errorf("invalid IP range: %s", rangeStr)
	}

	startIP = startIP.To4()
	endIP = endIP.To4()

	if startIP == nil || endIP == nil {
		return nil, fmt.Errorf("only IPv4 supported: %s", rangeStr)
	}

	var ips []string
	for ip := startIP; !ip.Equal(endIP); incrementIP(ip) {
		ips = append(ips, ip.String())
		if len(ips) > 1000 { // Safety limit
			break
		}
	}
	ips = append(ips, endIP.String())

	return ips, nil
}

// incrementIP increments an IP address by 1
func incrementIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] > 0 {
			break
		}
	}
}

// getUsedIPs returns a map of IPs currently in use by BOSH instances
func getUsedIPs() (map[string]bool, error) {
	cmd := exec.Command("bosh", "instances", "--json")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var result struct {
		Tables []struct {
			Rows []struct {
				IPs string `json:"ips"`
			} `json:"Rows"`
		} `json:"Tables"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, err
	}

	used := make(map[string]bool)
	for _, table := range result.Tables {
		for _, row := range table.Rows {
			for _, ip := range strings.Split(row.IPs, "\n") {
				ip = strings.TrimSpace(ip)
				if ip != "" {
					used[ip] = true
				}
			}
		}
	}

	return used, nil
}
