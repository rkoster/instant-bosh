package director

import (
	"context"
	"fmt"
	"os"
	"sync"

	boshdir "github.com/cloudfoundry/bosh-cli/v7/director"
	boshuaa "github.com/cloudfoundry/bosh-cli/v7/uaa"
	bosherr "github.com/cloudfoundry/bosh-utils/errors"
	boshhttp "github.com/cloudfoundry/bosh-utils/httpclient"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/rkoster/instant-bosh/internal/docker"
	"gopkg.in/yaml.v3"
)

// dialerMutex protects the global state manipulation in NewDirector
// (os.Unsetenv and boshhttp.ResetDialerContext)
var dialerMutex sync.Mutex

// Config holds the BOSH director connection configuration
type Config struct {
	Environment    string
	Client         string
	ClientSecret   string
	CACert         string
	AllProxy       string
	JumpboxKeyPath string
}

// ConfigProvider is an interface for retrieving BOSH director configuration.
// This allows for dependency injection and testing with fake config providers.
//
//go:generate counterfeiter . ConfigProvider
type ConfigProvider interface {
	GetDirectorConfig(ctx context.Context, dockerClient *docker.Client) (*Config, error)
}

// DefaultConfigProvider retrieves director config from the running container.
type DefaultConfigProvider struct{}

// GetDirectorConfig retrieves the BOSH director configuration from the running container.
func (p *DefaultConfigProvider) GetDirectorConfig(ctx context.Context, dockerClient *docker.Client) (*Config, error) {
	return GetDirectorConfig(ctx, dockerClient)
}

// Cleanup removes the temporary jumpbox key file
func (c *Config) Cleanup() error {
	if c.JumpboxKeyPath != "" {
		if err := os.Remove(c.JumpboxKeyPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove jumpbox key file: %w", err)
		}
	}
	return nil
}

// GetDirectorConfig retrieves the BOSH director configuration from the running container
func GetDirectorConfig(ctx context.Context, dockerClient *docker.Client) (*Config, error) {
	// Read vars-store.yml from container
	varsStore, err := dockerClient.ExecCommand(ctx, docker.ContainerName, []string{"cat", "/var/vcap/store/vars-store.yml"})
	if err != nil {
		return nil, fmt.Errorf("failed to read vars-store.yml: %w", err)
	}

	// Parse YAML
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(varsStore), &data); err != nil {
		return nil, fmt.Errorf("failed to parse vars-store.yml: %w", err)
	}

	// Extract admin password
	adminPassword, err := extractYAMLValue(data, "admin_password")
	if err != nil {
		return nil, fmt.Errorf("failed to extract admin_password: %w", err)
	}
	adminPasswordStr, ok := adminPassword.(string)
	if !ok {
		return nil, fmt.Errorf("admin_password is not a string")
	}

	// Extract director SSL CA certificate
	directorSSL, err := extractYAMLValue(data, "director_ssl")
	if err != nil {
		return nil, fmt.Errorf("failed to extract director_ssl: %w", err)
	}
	directorSSLMap, ok := directorSSL.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("director_ssl is not a map")
	}
	directorCert, err := extractYAMLValue(directorSSLMap, "ca")
	if err != nil {
		return nil, fmt.Errorf("failed to extract director_ssl.ca: %w", err)
	}
	directorCertStr, ok := directorCert.(string)
	if !ok {
		return nil, fmt.Errorf("director_ssl.ca is not a string")
	}

	// Extract jumpbox SSH key
	jumpboxSSH, err := extractYAMLValue(data, "jumpbox_ssh")
	if err != nil {
		return nil, fmt.Errorf("failed to extract jumpbox_ssh: %w", err)
	}
	jumpboxSSHMap, ok := jumpboxSSH.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("jumpbox_ssh is not a map")
	}
	jumpboxKey, err := extractYAMLValue(jumpboxSSHMap, "private_key")
	if err != nil {
		return nil, fmt.Errorf("failed to extract jumpbox_ssh.private_key: %w", err)
	}
	jumpboxKeyStr, ok := jumpboxKey.(string)
	if !ok {
		return nil, fmt.Errorf("jumpbox_ssh.private_key is not a string")
	}

	// Create temporary file for jumpbox key
	keyFileHandle, err := os.CreateTemp("", "jumpbox-key-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file for jumpbox key: %w", err)
	}
	keyFile := keyFileHandle.Name()

	if _, err := keyFileHandle.Write([]byte(jumpboxKeyStr)); err != nil {
		if closeErr := keyFileHandle.Close(); closeErr != nil {
			os.Remove(keyFile)
			return nil, fmt.Errorf("failed to close jumpbox key file after write error: %w", closeErr)
		}
		os.Remove(keyFile)
		return nil, fmt.Errorf("failed to write jumpbox key: %w", err)
	}

	if err := keyFileHandle.Close(); err != nil {
		os.Remove(keyFile)
		return nil, fmt.Errorf("failed to close jumpbox key file: %w", err)
	}

	// Set restrictive permissions
	if err := os.Chmod(keyFile, 0600); err != nil {
		os.Remove(keyFile)
		return nil, fmt.Errorf("failed to set permissions on jumpbox key: %w", err)
	}

	return &Config{
		Environment:    "https://127.0.0.1:25555",
		Client:         "admin",
		ClientSecret:   adminPasswordStr,
		CACert:         directorCertStr,
		AllProxy:       fmt.Sprintf("ssh+socks5://jumpbox@localhost:2222?private-key=%s", keyFile),
		JumpboxKeyPath: keyFile,
	}, nil
}

// NewDirector creates a BOSH director client using the provided configuration
func NewDirector(config *Config, logger boshlog.Logger) (boshdir.Director, error) {
	// Protect global state manipulation with a mutex for thread safety
	dialerMutex.Lock()

	// Unset BOSH_ALL_PROXY to ensure direct connection to localhost.
	// This variable is meant for external BOSH CLI usage through the jumpbox,
	// not for our internal API calls to localhost:25555.
	os.Unsetenv("BOSH_ALL_PROXY")

	// Reset the HTTP client dialer to pick up the environment variable change.
	// The bosh-utils library caches the dialer with proxy settings at package init time,
	// so we must explicitly reset it after unsetting BOSH_ALL_PROXY.
	boshhttp.ResetDialerContext()

	dialerMutex.Unlock()

	// Create director config
	directorConfig, err := boshdir.NewConfigFromURL(config.Environment)
	if err != nil {
		return nil, bosherr.WrapErrorf(err, "Building director config from URL '%s'", config.Environment)
	}

	directorConfig.CACert = config.CACert
	directorConfig.Client = config.Client
	directorConfig.ClientSecret = config.ClientSecret

	// Create UAA config for authentication
	uaaConfig, err := boshuaa.NewConfigFromURL(config.Environment)
	if err != nil {
		return nil, bosherr.WrapErrorf(err, "Building UAA config from URL '%s'", config.Environment)
	}
	uaaConfig.CACert = config.CACert
	uaaConfig.Client = config.Client
	uaaConfig.ClientSecret = config.ClientSecret

	// Create UAA client
	uaaFactory := boshuaa.NewFactory(logger)
	uaa, err := uaaFactory.New(uaaConfig)
	if err != nil {
		return nil, bosherr.WrapError(err, "Building UAA client")
	}

	// Create token function for authentication
	directorConfig.TokenFunc = boshuaa.NewClientTokenSession(uaa).TokenFunc

	// Create director factory
	directorFactory := boshdir.NewFactory(logger)

	// Create director client
	director, err := directorFactory.New(directorConfig, boshdir.NewNoopTaskReporter(), boshdir.NewNoopFileReporter())
	if err != nil {
		return nil, bosherr.WrapError(err, "Building director client")
	}

	return director, nil
}

func extractYAMLValue(data map[string]interface{}, key string) (interface{}, error) {
	value, ok := data[key]
	if !ok {
		return nil, fmt.Errorf("key '%s' not found", key)
	}
	return value, nil
}
