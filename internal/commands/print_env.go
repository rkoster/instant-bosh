package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	boshui "github.com/cloudfoundry/bosh-cli/v7/ui"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/docker"
	"gopkg.in/yaml.v3"
)

func PrintEnvAction(ui boshui.UI, logger boshlog.Logger) error {
	ctx := context.Background()

	dockerClient, err := docker.NewClient(logger)
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer dockerClient.Close()

	running, err := dockerClient.IsContainerRunning(ctx)
	if err != nil {
		return err
	}

	if !running {
		return fmt.Errorf("instant-bosh container is not running. Please run 'ibosh start' first")
	}

	// Read vars-store.yml from container
	varsStore, err := dockerClient.ExecCommand(ctx, docker.ContainerName, []string{"cat", "/var/vcap/store/vars-store.yml"})
	if err != nil {
		return fmt.Errorf("failed to read vars-store.yml: %w", err)
	}

	// Parse YAML
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(varsStore), &data); err != nil {
		return fmt.Errorf("failed to parse vars-store.yml: %w", err)
	}

	// Extract values
	adminPassword, err := extractYAMLValue(data, "admin_password")
	if err != nil {
		return fmt.Errorf("failed to extract admin_password: %w", err)
	}

	directorSSL, err := extractYAMLValue(data, "director_ssl")
	if err != nil {
		return fmt.Errorf("failed to extract director_ssl: %w", err)
	}
	directorSSLMap, ok := directorSSL.(map[string]interface{})
	if !ok {
		return fmt.Errorf("director_ssl is not a map")
	}
	directorCert, err := extractYAMLValue(directorSSLMap, "ca")
	if err != nil {
		return fmt.Errorf("failed to extract director_ssl.ca: %w", err)
	}

	jumpboxSSH, err := extractYAMLValue(data, "jumpbox_ssh")
	if err != nil {
		return fmt.Errorf("failed to extract jumpbox_ssh: %w", err)
	}
	jumpboxSSHMap, ok := jumpboxSSH.(map[string]interface{})
	if !ok {
		return fmt.Errorf("jumpbox_ssh is not a map")
	}
	jumpboxKey, err := extractYAMLValue(jumpboxSSHMap, "private_key")
	if err != nil {
		return fmt.Errorf("failed to extract jumpbox_ssh.private_key: %w", err)
	}

	// Create temporary file for jumpbox key
	tmpDir := os.TempDir()
	keyFile := filepath.Join(tmpDir, fmt.Sprintf("jumpbox-key-%d", os.Getpid()))

	if err := os.WriteFile(keyFile, []byte(fmt.Sprint(jumpboxKey)), 0600); err != nil {
		return fmt.Errorf("failed to write jumpbox key: %w", err)
	}

	// Print environment variables to stdout (must use ui.PrintLinef which goes to outWriter/stdout)
	ui.PrintLinef("export BOSH_CLIENT=admin")
	ui.PrintLinef("export BOSH_CLIENT_SECRET=%s", adminPassword)
	ui.PrintLinef("export BOSH_ENVIRONMENT=https://127.0.0.1:25555")
	ui.PrintLinef("export BOSH_CA_CERT='%s'", directorCert)
	ui.PrintLinef("export BOSH_ALL_PROXY=ssh+socks5://jumpbox@localhost:2222?private-key=%s", keyFile)

	return nil
}

func extractYAMLValue(data map[string]interface{}, key string) (interface{}, error) {
	value, ok := data[key]
	if !ok {
		return nil, fmt.Errorf("key %s not found", key)
	}
	return value, nil
}
