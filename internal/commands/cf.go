package commands

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/rkoster/instant-bosh/internal/configserver"
	"github.com/rkoster/instant-bosh/internal/manifests"
	"gopkg.in/yaml.v3"
)

// CFDeployOptions contains options for CF deployment
type CFDeployOptions struct {
	RouterIP     string // Optional: specify router IP, otherwise auto-select
	SystemDomain string // Optional: specify system domain, otherwise derive from router IP
	DryRun       bool   // If true, show what would be deployed without deploying
}

// CFDeployAction deploys Cloud Foundry to the BOSH director
func CFDeployAction(ui UI, opts CFDeployOptions) error {
	// Check that BOSH env vars are set
	if err := checkBOSHEnv(); err != nil {
		return err
	}

	// Determine router IP
	routerIP := opts.RouterIP
	if routerIP == "" {
		var err error
		routerIP, err = selectRouterIP(ui)
		if err != nil {
			return fmt.Errorf("failed to select router IP: %w", err)
		}
	}

	// Determine system domain
	systemDomain := opts.SystemDomain
	if systemDomain == "" {
		systemDomain = fmt.Sprintf("%s.sslip.io", routerIP)
	}

	ui.PrintLinef("Deploying CF with:")
	ui.PrintLinef("  Router IP:     %s", routerIP)
	ui.PrintLinef("  System Domain: %s", systemDomain)
	ui.PrintLinef("")

	if opts.DryRun {
		ui.PrintLinef("Dry run - would deploy with these settings")
		return nil
	}

	// Create temporary directory for manifest files
	tmpDir, err := os.MkdirTemp("", "cf-deploy-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write main manifest
	manifest, err := manifests.CFDeploymentManifest()
	if err != nil {
		return fmt.Errorf("failed to read cf-deployment manifest: %w", err)
	}
	manifestPath := tmpDir + "/cf-deployment.yml"
	if err := os.WriteFile(manifestPath, manifest, 0644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	// Write ops files
	opsFiles, err := manifests.StandardCFOpsFiles()
	if err != nil {
		return fmt.Errorf("failed to read ops files: %w", err)
	}

	opsFilePaths := []string{
		"scale-to-one-az.yml",
		"use-compiled-releases.yml",
		"set-router-static-ips.yml",
		"fast-deploy-with-downtime-and-danger.yml",
		"use-create-swap-delete-vm-strategy.yml",
	}

	var opsPaths []string
	for i, content := range opsFiles {
		path := tmpDir + "/" + opsFilePaths[i]
		if err := os.WriteFile(path, content, 0644); err != nil {
			return fmt.Errorf("failed to write ops file %s: %w", opsFilePaths[i], err)
		}
		opsPaths = append(opsPaths, path)
	}

	// Build bosh deploy command
	args := []string{
		"deploy", manifestPath,
		"-d", "cf",
		"-n", // non-interactive
		"-v", fmt.Sprintf("system_domain=%s", systemDomain),
		"-v", fmt.Sprintf("router_static_ips=[%s]", routerIP),
	}

	for _, opsPath := range opsPaths {
		args = append(args, "-o", opsPath)
	}

	ui.PrintLinef("Running: bosh %s", strings.Join(args, " "))
	ui.PrintLinef("")

	cmd := exec.Command("bosh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bosh deploy failed: %w", err)
	}

	ui.PrintLinef("")
	ui.PrintLinef("CF deployment complete!")
	ui.PrintLinef("")
	ui.PrintLinef("To configure your CF CLI:")
	ui.PrintLinef("  eval \"$(ibosh cf print-env)\"")
	ui.PrintLinef("")
	ui.PrintLinef("Or login directly:")
	ui.PrintLinef("  ibosh cf login")

	return nil
}

// CFDeleteAction deletes the CF deployment
func CFDeleteAction(ui UI, force bool) error {
	// Check that BOSH env vars are set
	if err := checkBOSHEnv(); err != nil {
		return err
	}

	args := []string{"delete-deployment", "-d", "cf"}
	if force {
		args = append(args, "-n") // non-interactive
	}

	ui.PrintLinef("Deleting CF deployment...")

	cmd := exec.Command("bosh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bosh delete-deployment failed: %w", err)
	}

	ui.PrintLinef("CF deployment deleted")
	return nil
}

// CFPrintEnvAction prints environment variables for CF CLI
func CFPrintEnvAction(ui UI) error {
	// Get config-server client
	client, err := configserver.NewClientFromEnv()
	if err != nil {
		return err
	}

	// Get system domain from the deployment (router IP based)
	// For now, we'll try to get the CF API from the router instance
	// The admin password is stored in config-server as /cf/cf_admin_password or /cf/uaa_admin_client_secret

	// Try to find the system domain by checking deployed CF
	systemDomain, err := getCFSystemDomain()
	if err != nil {
		return fmt.Errorf("failed to get CF system domain: %w\nIs CF deployed? Run 'ibosh cf deploy' first.", err)
	}

	// Get admin password from config-server
	// Config-server credentials are stored as /<director-name>/<deployment-name>/<variable-name>
	// Director name is "instant-bosh"
	adminCred, err := client.Get("/instant-bosh/cf/cf_admin_password")
	if err != nil {
		// Try alternate credential name
		adminCred, err = client.Get("/instant-bosh/cf/uaa_admin_client_secret")
		if err != nil {
			return fmt.Errorf("failed to get CF admin credentials from config-server: %w", err)
		}
	}

	adminPassword, ok := adminCred.Value.(string)
	if !ok {
		return fmt.Errorf("unexpected credential type for admin password")
	}

	cfAPI := fmt.Sprintf("https://api.%s", systemDomain)

	// Print as shell exports
	fmt.Printf("export CF_API='%s'\n", cfAPI)
	fmt.Printf("export CF_USERNAME='admin'\n")
	fmt.Printf("export CF_PASSWORD='%s'\n", adminPassword)
	fmt.Printf("export CF_SKIP_SSL_VALIDATION='true'\n")

	return nil
}

// CFLoginAction logs into CF using the CLI
func CFLoginAction(ui UI) error {
	// Get config-server client
	client, err := configserver.NewClientFromEnv()
	if err != nil {
		return err
	}

	// Get system domain
	systemDomain, err := getCFSystemDomain()
	if err != nil {
		return fmt.Errorf("failed to get CF system domain: %w\nIs CF deployed? Run 'ibosh cf deploy' first.", err)
	}

	// Get admin password
	// Config-server credentials are stored as /<director-name>/<deployment-name>/<variable-name>
	adminCred, err := client.Get("/instant-bosh/cf/cf_admin_password")
	if err != nil {
		// Try alternate credential name
		adminCred, err = client.Get("/instant-bosh/cf/uaa_admin_client_secret")
		if err != nil {
			return fmt.Errorf("failed to get CF admin credentials: %w", err)
		}
	}

	adminPassword, ok := adminCred.Value.(string)
	if !ok {
		return fmt.Errorf("unexpected credential type for admin password")
	}

	cfAPI := fmt.Sprintf("https://api.%s", systemDomain)

	ui.PrintLinef("Targeting CF API: %s", cfAPI)

	// Run cf api
	apiCmd := exec.Command("cf", "api", cfAPI, "--skip-ssl-validation")
	apiCmd.Stdout = os.Stdout
	apiCmd.Stderr = os.Stderr
	if err := apiCmd.Run(); err != nil {
		return fmt.Errorf("cf api failed: %w", err)
	}

	ui.PrintLinef("Authenticating as admin...")

	// Run cf auth
	authCmd := exec.Command("cf", "auth", "admin", adminPassword)
	authCmd.Stdout = os.Stdout
	authCmd.Stderr = os.Stderr
	if err := authCmd.Run(); err != nil {
		return fmt.Errorf("cf auth failed: %w", err)
	}

	ui.PrintLinef("")
	ui.PrintLinef("Successfully logged in to CF!")
	ui.PrintLinef("")
	ui.PrintLinef("To create an org and space:")
	ui.PrintLinef("  cf create-org my-org")
	ui.PrintLinef("  cf target -o my-org")
	ui.PrintLinef("  cf create-space my-space")
	ui.PrintLinef("  cf target -s my-space")

	return nil
}

// checkBOSHEnv verifies that required BOSH environment variables are set
func checkBOSHEnv() error {
	required := []string{"BOSH_ENVIRONMENT", "BOSH_CLIENT", "BOSH_CLIENT_SECRET", "BOSH_CA_CERT"}
	var missing []string

	for _, v := range required {
		if os.Getenv(v) == "" {
			missing = append(missing, v)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("BOSH environment not configured. Missing: %s\nRun: eval \"$(ibosh docker print-env)\" or eval \"$(ibosh incus print-env)\"",
			strings.Join(missing, ", "))
	}

	return nil
}

// selectRouterIP selects an available IP from the cloud-config static range
func selectRouterIP(ui UI) (string, error) {
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

	// Find static IPs from default network
	var staticRange string
	for _, network := range cloudConfig.Networks {
		if network.Name == "default" && len(network.Subnets) > 0 && len(network.Subnets[0].Static) > 0 {
			staticRange = network.Subnets[0].Static[0]
			break
		}
	}

	if staticRange == "" {
		return "", fmt.Errorf("no static IP range found in cloud-config")
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
		ui.PrintLinef("Warning: could not get used IPs, may conflict: %v", err)
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

// getCFSystemDomain determines the CF system domain from the deployment
func getCFSystemDomain() (string, error) {
	// Try to get from the CF deployment's router instance
	cmd := exec.Command("bosh", "-d", "cf", "instances", "--json")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get CF instances: %w", err)
	}

	var result struct {
		Tables []struct {
			Rows []struct {
				Instance string `json:"instance"`
				IPs      string `json:"ips"`
			} `json:"Rows"`
		} `json:"Tables"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return "", err
	}

	// Find router instance IP
	for _, table := range result.Tables {
		for _, row := range table.Rows {
			if strings.HasPrefix(row.Instance, "router/") {
				ip := strings.TrimSpace(strings.Split(row.IPs, "\n")[0])
				if ip != "" {
					return fmt.Sprintf("%s.sslip.io", ip), nil
				}
			}
		}
	}

	return "", fmt.Errorf("router instance not found in CF deployment")
}
