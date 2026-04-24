package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/configserver"
	"github.com/rkoster/instant-bosh/internal/cpi"
	"github.com/rkoster/instant-bosh/internal/director"
	"github.com/rkoster/instant-bosh/internal/manifests"
	"github.com/rkoster/instant-bosh/internal/uploadedrelease"
)

// BOSHDeployConfig holds the configuration for deploying a BOSH director
type BOSHDeployConfig struct {
	CPI        string // CPI type (only "warden" supported initially)
	DirectorIP string // Director IP address (auto-selected if empty)
	DryRun     bool   // If true, only prepare manifest without deploying
}

// BOSHDeployAction deploys a BOSH director as a BOSH deployment
func BOSHDeployAction(ui UI, cpiType, directorIP string, dryRun bool) error {
	ctx := context.Background()

	// Validate that we're using Incus backend (required for warden)
	detectedCPI, err := cpi.DetectCPIType(ctx)
	if err != nil {
		return fmt.Errorf("detecting CPI type: %w", err)
	}
	if detectedCPI != cpi.CPITypeIncus {
		return fmt.Errorf("BOSH deployment with warden CPI requires Incus backend (detected: %s)", detectedCPI)
	}

	// Resolve configuration
	config, err := ResolveBOSHConfig(ui, cpiType, directorIP)
	if err != nil {
		return fmt.Errorf("resolving BOSH config: %w", err)
	}

	deploymentName := fmt.Sprintf("bosh-%s", config.CPI)

	ui.PrintLinef("Deploying BOSH director '%s' with %s CPI", deploymentName, config.CPI)
	ui.PrintLinef("Director IP: %s", config.DirectorIP)

	// Prepare manifest and ops files
	manifestFiles, err := PrepareBOSHManifestFiles(config)
	if err != nil {
		return fmt.Errorf("preparing manifest files: %w", err)
	}
	defer manifestFiles.Cleanup()

	// Interpolate manifest with variables
	interpolatedManifest, err := interpolateBOSHManifest(manifestFiles, config)
	if err != nil {
		return fmt.Errorf("interpolating manifest: %w", err)
	}

	if dryRun {
		ui.PrintLinef("\n--- Interpolated Manifest (dry-run) ---")
		fmt.Println(string(interpolatedManifest))
		ui.PrintLinef("\nDry-run complete. Manifest would be deployed to deployment '%s'", deploymentName)
		return nil
	}

	// Filter the manifest to remove url/sha1 from already-uploaded releases
	ui.PrintLinef("Checking for already uploaded releases...")

	// Save BOSH env vars before creating clients (createDirectorClient unsets them)
	savedEnv := saveBOSHEnvVars()

	cpiInstance, cpiCleanup, err := createCPIAndDirectorClient(ctx)
	if err != nil {
		restoreBOSHEnvVars(savedEnv)
		return fmt.Errorf("failed to create CPI client: %w", err)
	}
	defer cpiCleanup()

	directorClient, directorCleanup, err := createDirectorClient(ctx, cpiInstance)
	if err != nil {
		restoreBOSHEnvVars(savedEnv)
		return fmt.Errorf("failed to create director client: %w", err)
	}
	defer directorCleanup()

	filteredManifest, err := uploadedrelease.Filter(interpolatedManifest, directorClient)
	if err != nil {
		restoreBOSHEnvVars(savedEnv)
		return fmt.Errorf("failed to filter releases: %w", err)
	}

	// Restore env vars before running bosh deploy (it needs them)
	restoreBOSHEnvVars(savedEnv)

	// Deploy the BOSH director with filtered manifest
	if err := deployBOSHDirector(ui, deploymentName, filteredManifest, config); err != nil {
		return fmt.Errorf("deploying BOSH director: %w", err)
	}

	// Apply warden cloud-config to the deployed director
	ui.PrintLinef("\nApplying warden cloud-config to director...")
	logger := boshlog.NewLogger(boshlog.LevelError)
	if err := applyWardenCloudConfigToDeployedDirector(ui, config, logger); err != nil {
		ui.PrintLinef("Warning: Failed to apply warden cloud-config: %v", err)
		ui.PrintLinef("You can manually apply it later by targeting the warden director:")
		ui.PrintLinef("  bosh -e bosh-%s update-cloud-config <path-to-warden-cloud-config.yml>", config.CPI)
		// Don't fail the deployment - just warn
	} else {
		ui.PrintLinef("✓ Warden cloud-config applied successfully")
	}

	// Print success message with instructions
	ui.PrintLinef("\n✓ BOSH director '%s' deployed successfully!", deploymentName)
	ui.PrintLinef("\nThe warden cloud-config has been applied to the director.")
	ui.PrintLinef("\nTo target this director:")
	ui.PrintLinef("  bosh alias-env bosh-%s -e %s --ca-cert <(bosh int <(credhub get -n /instant-bosh/%s/director_ssl -k ca) --path /ca)", config.CPI, config.DirectorIP, deploymentName)
	ui.PrintLinef("  export BOSH_CLIENT=admin")
	ui.PrintLinef("  export BOSH_CLIENT_SECRET=$(credhub get -n /instant-bosh/%s/admin_password -q)", deploymentName)
	ui.PrintLinef("  bosh -e bosh-%s env", config.CPI)
	ui.PrintLinef("\nTo upload a dev release:")
	ui.PrintLinef("  bosh -e bosh-%s upload-release /path/to/dev-release.tgz", config.CPI)
	ui.PrintLinef("\nTo update the director with changes:")
	ui.PrintLinef("  ibosh bosh deploy --cpi %s", config.CPI)

	return nil
}

// BOSHDeleteAction deletes a BOSH director deployment
func BOSHDeleteAction(ui UI, cpiType string, force bool) error {
	if cpiType == "" {
		cpiType = "warden"
	}

	// Validate CPI
	if cpiType != "warden" {
		return fmt.Errorf("unsupported CPI type: %s (only 'warden' is supported)", cpiType)
	}

	deploymentName := fmt.Sprintf("bosh-%s", cpiType)

	ui.PrintLinef("Deleting BOSH director deployment '%s'", deploymentName)

	// Build bosh delete-deployment command
	args := []string{"-d", deploymentName, "delete-deployment"}
	if force {
		args = append(args, "--force")
	}

	cmd := exec.Command("bosh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	ui.PrintLinef("\n✓ BOSH director '%s' deleted successfully", deploymentName)

	return nil
}

// BOSHPrintEnvAction prints environment variables for targeting a deployed BOSH director
func BOSHPrintEnvAction(ui UI, cpiType string) error {
	if cpiType == "" {
		cpiType = "warden"
	}
	if cpiType != "warden" {
		return fmt.Errorf("unsupported CPI type: %s (only 'warden' is supported)", cpiType)
	}

	deploymentName := fmt.Sprintf("bosh-%s", cpiType)

	// Get director IP from existing deployment
	directorIP, err := getExistingBOSHDirectorIP(deploymentName)
	if err != nil {
		return fmt.Errorf("failed to get director IP: %w\nIs the BOSH director deployed? Run 'ibosh bosh deploy' first.", err)
	}

	// Get credentials from config-server
	configClient, err := configserver.NewClientFromEnv()
	if err != nil {
		return err
	}

	// Fetch admin password
	adminPasswordCred, err := configClient.Get(fmt.Sprintf("/instant-bosh/%s/admin_password", deploymentName))
	if err != nil {
		return fmt.Errorf("fetching admin password: %w", err)
	}
	password, ok := adminPasswordCred.Value.(string)
	if !ok {
		return fmt.Errorf("admin_password is not a string")
	}

	// Fetch director SSL CA cert
	directorSSLCred, err := configClient.Get(fmt.Sprintf("/instant-bosh/%s/director_ssl", deploymentName))
	if err != nil {
		return fmt.Errorf("fetching director_ssl: %w", err)
	}
	sslMap, ok := directorSSLCred.Value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("director_ssl is not a map")
	}
	caCert, ok := sslMap["ca"].(string)
	if !ok {
		return fmt.Errorf("director_ssl.ca is not a string")
	}

	// Fetch jumpbox SSH key and write to a stable path
	jumpboxSSHCred, err := configClient.Get(fmt.Sprintf("/instant-bosh/%s/jumpbox_ssh", deploymentName))
	if err != nil {
		return fmt.Errorf("fetching jumpbox_ssh: %w", err)
	}
	sshMap, ok := jumpboxSSHCred.Value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("jumpbox_ssh is not a map")
	}
	privateKey, ok := sshMap["private_key"].(string)
	if !ok {
		return fmt.Errorf("jumpbox_ssh.private_key is not a string")
	}

	// Write jumpbox key to a stable path (not a temp file that gets cleaned up)
	keyPath := filepath.Join(os.TempDir(), fmt.Sprintf("ibosh-%s-jumpbox-key", deploymentName))
	if err := os.WriteFile(keyPath, []byte(privateKey), 0600); err != nil {
		return fmt.Errorf("writing jumpbox key: %w", err)
	}

	allProxy := fmt.Sprintf("ssh+socks5://jumpbox@%s:22?private-key=%s", directorIP, keyPath)

	// Print as shell exports
	fmt.Printf("export BOSH_ENVIRONMENT=%s\n", fmt.Sprintf("https://%s:25555", directorIP))
	fmt.Printf("export BOSH_CLIENT=%s\n", "admin")
	fmt.Printf("export BOSH_CLIENT_SECRET=%s\n", password)
	fmt.Printf("export BOSH_CA_CERT='%s'\n", caCert)
	fmt.Printf("export BOSH_ALL_PROXY=%s\n", allProxy)

	return nil
}

// ResolveBOSHConfig resolves and validates the BOSH deployment configuration
func ResolveBOSHConfig(ui UI, cpiType, directorIP string) (*BOSHDeployConfig, error) {
	// Set default CPI if not specified
	if cpiType == "" {
		cpiType = "warden"
	}

	// Validate CPI type
	if cpiType != "warden" {
		return nil, fmt.Errorf("unsupported CPI type: %s (only 'warden' is supported)", cpiType)
	}

	// Resolve director IP
	var resolvedIP string
	if directorIP != "" {
		resolvedIP = directorIP
		ui.PrintLinef("Using provided director IP: %s", resolvedIP)
	} else {
		// Try to get existing director IP from deployment
		deploymentName := fmt.Sprintf("bosh-%s", cpiType)
		existingIP, err := getExistingBOSHDirectorIP(deploymentName)
		if err == nil && existingIP != "" {
			resolvedIP = existingIP
			ui.PrintLinef("Using existing director IP from deployment: %s", resolvedIP)
		} else {
			// Auto-select from cloud-config (network 10.244.0.0/24 for warden)
			ipSelector := NewIPSelector(ui)
			resolvedIP, err = ipSelector.SelectAvailableIP("default")
			if err != nil {
				return nil, fmt.Errorf("failed to select director IP: %w", err)
			}
			ui.PrintLinef("Auto-selected director IP: %s", resolvedIP)
		}
	}

	return &BOSHDeployConfig{
		CPI:        cpiType,
		DirectorIP: resolvedIP,
	}, nil
}

// BOSHManifestFiles holds temporary files for BOSH manifest preparation
type BOSHManifestFiles struct {
	BaseManifest string
	OpsFiles     []string
	TempDir      string
}

// Cleanup removes temporary files
func (m *BOSHManifestFiles) Cleanup() {
	if m.TempDir != "" {
		os.RemoveAll(m.TempDir)
	}
}

// PrepareBOSHManifestFiles prepares the base manifest and ops files
func PrepareBOSHManifestFiles(config *BOSHDeployConfig) (*BOSHManifestFiles, error) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "ibosh-bosh-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp directory: %w", err)
	}

	result := &BOSHManifestFiles{
		TempDir: tempDir,
	}

	// Write base manifest
	baseManifest, err := manifests.BOSHDeploymentManifest()
	if err != nil {
		result.Cleanup()
		return nil, fmt.Errorf("reading base manifest: %w", err)
	}
	result.BaseManifest = filepath.Join(tempDir, "bosh.yml")
	if err := os.WriteFile(result.BaseManifest, baseManifest, 0644); err != nil {
		result.Cleanup()
		return nil, fmt.Errorf("writing base manifest: %w", err)
	}

	// Prepare ops files in the correct order for warden
	opsFileFuncs := []struct {
		name string
		fn   func() ([]byte, error)
	}{
		{"bosh-lite.yml", manifests.BOSHLiteManifest},
		{"warden-cpi.yml", manifests.WardenCPIOpsFile},
		{"jumpbox-user.yml", manifests.JumpboxUserOpsFile},
		{"uaa.yml", manifests.UAAOpsFile},
		{"credhub.yml", manifests.CredhubOpsFile},
		{"bosh-dev.yml", manifests.BOSHDevOpsFile}, // KEY: Converts create-env to deploy format
	}

	for _, opsFile := range opsFileFuncs {
		content, err := opsFile.fn()
		if err != nil {
			result.Cleanup()
			return nil, fmt.Errorf("reading ops file %s: %w", opsFile.name, err)
		}
		opsPath := filepath.Join(tempDir, opsFile.name)
		if err := os.WriteFile(opsPath, content, 0644); err != nil {
			result.Cleanup()
			return nil, fmt.Errorf("writing ops file %s: %w", opsFile.name, err)
		}
		result.OpsFiles = append(result.OpsFiles, opsPath)
	}

	return result, nil
}

// interpolateBOSHManifest interpolates the manifest with variables using bosh interpolate
func interpolateBOSHManifest(manifestFiles *BOSHManifestFiles, config *BOSHDeployConfig) ([]byte, error) {
	deploymentName := fmt.Sprintf("bosh-%s", config.CPI)

	// Create a temporary ops file to set the deployment name
	nameOpsFile := filepath.Join(manifestFiles.TempDir, "set-deployment-name.yml")
	nameOpsContent := fmt.Sprintf(`- type: replace
  path: /name
  value: %s
`, deploymentName)
	if err := os.WriteFile(nameOpsFile, []byte(nameOpsContent), 0644); err != nil {
		return nil, fmt.Errorf("writing deployment name ops file: %w", err)
	}

	// Build bosh interpolate command
	args := []string{"interpolate", manifestFiles.BaseManifest}

	// Add deployment name ops file first
	args = append(args, "-o", nameOpsFile)

	// Add other ops files
	for _, opsFile := range manifestFiles.OpsFiles {
		args = append(args, "-o", opsFile)
	}

	// Add variables
	args = append(args,
		"-v", fmt.Sprintf("internal_ip=%s", config.DirectorIP),
		"-v", "internal_cidr=10.244.0.0/24",
		"-v", "internal_gw=10.244.0.1",
		"-v", "garden_host=127.0.0.1", // Always localhost for warden
		"-v", fmt.Sprintf("director_name=%s", deploymentName),
	)

	cmd := exec.Command("bosh", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("interpolating manifest: %w\nOutput: %s", err, string(output))
	}

	return output, nil
}

// deployBOSHDirector deploys the BOSH director using bosh deploy
func deployBOSHDirector(ui UI, deploymentName string, manifest []byte, config *BOSHDeployConfig) error {
	// Write manifest to temp file
	tempManifest := filepath.Join(os.TempDir(), fmt.Sprintf("bosh-%s-manifest.yml", config.CPI))
	if err := os.WriteFile(tempManifest, manifest, 0644); err != nil {
		return fmt.Errorf("writing manifest file: %w", err)
	}
	defer os.Remove(tempManifest)

	// Build bosh deploy command
	// The BOSH director will automatically use config-server for variable interpolation
	args := []string{
		"-n",
		"-d", deploymentName,
		"deploy", tempManifest,
	}

	ui.PrintLinef("\nRunning: bosh %s", strings.Join(args, " "))

	cmd := exec.Command("bosh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bosh deploy failed: %w", err)
	}

	return nil
}

// getWardenDirectorConfigFromConfigServer retrieves director configuration from config-server
// for a deployed warden BOSH director
func getWardenDirectorConfigFromConfigServer(deploymentName string, directorIP string, logger boshlog.Logger) (*director.Config, error) {
	// Create config-server client from environment variables
	configClient, err := configserver.NewClientFromEnv()
	if err != nil {
		return nil, fmt.Errorf("creating config-server client: %w", err)
	}

	// Fetch admin password
	adminPasswordCred, err := configClient.Get(fmt.Sprintf("/instant-bosh/%s/admin_password", deploymentName))
	if err != nil {
		return nil, fmt.Errorf("fetching admin password: %w", err)
	}
	password, ok := adminPasswordCred.Value.(string)
	if !ok {
		return nil, fmt.Errorf("admin_password is not a string")
	}

	// Fetch director SSL certificate
	directorSSLCred, err := configClient.Get(fmt.Sprintf("/instant-bosh/%s/director_ssl", deploymentName))
	if err != nil {
		return nil, fmt.Errorf("fetching director_ssl: %w", err)
	}
	sslMap, ok := directorSSLCred.Value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("director_ssl is not a map")
	}
	caCert, ok := sslMap["ca"].(string)
	if !ok {
		return nil, fmt.Errorf("director_ssl.ca is not a string")
	}

	// Fetch jumpbox SSH key
	jumpboxSSHCred, err := configClient.Get(fmt.Sprintf("/instant-bosh/%s/jumpbox_ssh", deploymentName))
	if err != nil {
		return nil, fmt.Errorf("fetching jumpbox_ssh: %w", err)
	}
	sshMap, ok := jumpboxSSHCred.Value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("jumpbox_ssh is not a map")
	}
	privateKey, ok := sshMap["private_key"].(string)
	if !ok {
		return nil, fmt.Errorf("jumpbox_ssh.private_key is not a string")
	}

	// Create temporary file for jumpbox key
	keyFileHandle, err := os.CreateTemp("", "warden-jumpbox-key-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp file for jumpbox key: %w", err)
	}
	keyFile := keyFileHandle.Name()

	if _, err := keyFileHandle.Write([]byte(privateKey)); err != nil {
		keyFileHandle.Close()
		os.Remove(keyFile)
		return nil, fmt.Errorf("writing jumpbox key: %w", err)
	}

	if err := keyFileHandle.Close(); err != nil {
		os.Remove(keyFile)
		return nil, fmt.Errorf("closing jumpbox key file: %w", err)
	}

	// Set restrictive permissions
	if err := os.Chmod(keyFile, 0600); err != nil {
		os.Remove(keyFile)
		return nil, fmt.Errorf("setting permissions on jumpbox key: %w", err)
	}

	// Build AllProxy string for SSH access to warden containers
	// Note: Port 22 (standard SSH) for deployed VM, not 2222 like instant-bosh container
	allProxy := fmt.Sprintf("ssh+socks5://jumpbox@%s:22?private-key=%s", directorIP, keyFile)

	return &director.Config{
		Environment:    fmt.Sprintf("https://%s:25555", directorIP),
		Client:         "admin",
		ClientSecret:   password,
		CACert:         caCert,
		UAAURL:         fmt.Sprintf("https://%s:8443", directorIP),
		UAACACert:      caCert,
		AllProxy:       allProxy,
		JumpboxKeyPath: keyFile,
	}, nil
}

// applyWardenCloudConfigToDeployedDirector applies the warden cloud-config to the deployed warden director
func applyWardenCloudConfigToDeployedDirector(ui UI, config *BOSHDeployConfig, logger boshlog.Logger) error {
	deploymentName := fmt.Sprintf("bosh-%s", config.CPI)

	// Get director configuration from config-server
	directorConfig, err := getWardenDirectorConfigFromConfigServer(deploymentName, config.DirectorIP, logger)
	if err != nil {
		return fmt.Errorf("getting director config from config-server: %w", err)
	}
	defer directorConfig.Cleanup()

	// Create director client
	directorClient, err := director.NewDirector(directorConfig, logger)
	if err != nil {
		return fmt.Errorf("creating director client: %w", err)
	}

	// Get warden cloud-config bytes
	cloudConfigBytes, err := manifests.WardenCloudConfig()
	if err != nil {
		return fmt.Errorf("reading warden cloud-config: %w", err)
	}

	// Apply cloud-config to the warden director
	if err := directorClient.UpdateCloudConfig("default", cloudConfigBytes); err != nil {
		return fmt.Errorf("updating cloud-config: %w", err)
	}

	return nil
}

// getExistingBOSHDirectorIP returns the director IP from an existing BOSH deployment, if any
func getExistingBOSHDirectorIP(deploymentName string) (string, error) {
	// Try to get from the BOSH deployment's director instance
	// Clear BOSH_ALL_PROXY since this command targets the instant-bosh director directly,
	// not through a SOCKS proxy (which may be set from a previous 'bosh print-env' eval)
	cmd := exec.Command("bosh", "-n", "-d", deploymentName, "instances", "--json")
	cmd.Env = filterEnv(os.Environ(), "BOSH_ALL_PROXY")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// bosh CLI outputs errors as JSON to stdout when --json is used
		var errResult struct {
			Lines []string `json:"Lines"`
		}
		if jsonErr := json.Unmarshal(output, &errResult); jsonErr == nil && len(errResult.Lines) > 0 {
			return "", fmt.Errorf("failed to get BOSH instances: %s", strings.Join(errResult.Lines, "; "))
		}
		return "", fmt.Errorf("failed to get BOSH instances: %w\n%s", err, string(output))
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

	// Find bosh instance IP
	for _, table := range result.Tables {
		for _, row := range table.Rows {
			if strings.HasPrefix(row.Instance, "bosh/") {
				ips := strings.Split(row.IPs, "\n")
				if len(ips) > 0 && ips[0] != "" {
					return strings.TrimSpace(ips[0]), nil
				}
			}
		}
	}

	return "", fmt.Errorf("no bosh instance found in deployment")
}

// filterEnv returns a copy of env with the specified keys removed
func filterEnv(env []string, keys ...string) []string {
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		exclude := false
		for _, key := range keys {
			if strings.HasPrefix(e, key+"=") {
				exclude = true
				break
			}
		}
		if !exclude {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
