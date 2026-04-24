package boshio

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	// DefaultBaseURL is the default bosh.io API base URL
	DefaultBaseURL = "https://bosh.io/api/v1"

	// DefaultTimeout is the default HTTP client timeout
	DefaultTimeout = 30 * time.Second
)

// StemcellInfo contains stemcell metadata from bosh.io API
type StemcellInfo struct {
	Name    string
	Version string
	URL     string
	SHA1    string
	Size    int64
}

// stemcellResponse represents the JSON response from bosh.io API
type stemcellResponse struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Regular *struct {
		URL    string `json:"url"`
		SHA1   string `json:"sha1"`
		SHA256 string `json:"sha256"`
		Size   int64  `json:"size"`
	} `json:"regular"`
	Light *struct {
		URL    string `json:"url"`
		SHA1   string `json:"sha1"`
		SHA256 string `json:"sha256"`
		Size   int64  `json:"size"`
	} `json:"light"`
}

// Client interacts with the bosh.io API
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// ClientOption is a function that configures the Client
type ClientOption func(*Client)

// WithBaseURL sets a custom base URL for the client
func WithBaseURL(url string) ClientOption {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// NewClient creates a new bosh.io API client
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		baseURL: DefaultBaseURL,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// ResolveStemcell resolves stemcell metadata from bosh.io
// For version="latest", returns the most recent version
// stemcellName is the full stemcell name, e.g., "bosh-openstack-kvm-ubuntu-jammy-go_agent"
func (c *Client) ResolveStemcell(ctx context.Context, stemcellName, version string) (*StemcellInfo, error) {
	return c.resolveStemcellWithName(ctx, stemcellName, version)
}

// resolveStemcellWithName resolves stemcell metadata for a specific stemcell name
func (c *Client) resolveStemcellWithName(ctx context.Context, stemcellName, version string) (*StemcellInfo, error) {
	url := fmt.Sprintf("%s/stemcells/%s", c.baseURL, stemcellName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching stemcell info from bosh.io: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("stemcell %q not found on bosh.io", stemcellName)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bosh.io API returned status %d", resp.StatusCode)
	}

	var stemcells []stemcellResponse
	if err := json.NewDecoder(resp.Body).Decode(&stemcells); err != nil {
		return nil, fmt.Errorf("decoding bosh.io response: %w", err)
	}

	if len(stemcells) == 0 {
		return nil, fmt.Errorf("no versions found for stemcell %q", stemcellName)
	}

	// Find the requested version
	var selected *stemcellResponse
	if version == "latest" || version == "" {
		// First entry is the latest (API returns sorted by version descending)
		selected = &stemcells[0]
	} else {
		for i := range stemcells {
			if stemcells[i].Version == version {
				selected = &stemcells[i]
				break
			}
		}
		if selected == nil {
			return nil, fmt.Errorf("version %q not found for stemcell %q", version, stemcellName)
		}
	}

	// Prefer regular stemcell (full), fall back to light if available
	if selected.Regular != nil {
		return &StemcellInfo{
			Name:    selected.Name,
			Version: selected.Version,
			URL:     selected.Regular.URL,
			SHA1:    selected.Regular.SHA1,
			Size:    selected.Regular.Size,
		}, nil
	}

	if selected.Light != nil {
		return &StemcellInfo{
			Name:    selected.Name,
			Version: selected.Version,
			URL:     selected.Light.URL,
			SHA1:    selected.Light.SHA1,
			Size:    selected.Light.Size,
		}, nil
	}

	return nil, fmt.Errorf("no download URL found for stemcell %q version %q", stemcellName, selected.Version)
}

// ResolveStemcellByInfra resolves a stemcell by infrastructure and OS name from bosh.io.
// It tries without the -go_agent suffix first (for newer stemcells like Noble),
// and falls back to with the suffix (for older stemcells like Jammy).
// infrastructure is the bosh.io stemcell infrastructure prefix, e.g.:
//   - "warden-boshlite" for warden CPI
//   - "openstack-kvm" for openstack/incus CPI
func (c *Client) ResolveStemcellByInfra(ctx context.Context, infrastructure, osName, version string) (*StemcellInfo, error) {
	stemcellName := fmt.Sprintf("bosh-%s-%s", infrastructure, osName)
	info, err := c.resolveStemcellWithName(ctx, stemcellName, version)
	if err == nil {
		return info, nil
	}

	stemcellNameWithAgent := fmt.Sprintf("bosh-%s-%s-go_agent", infrastructure, osName)
	return c.resolveStemcellWithName(ctx, stemcellNameWithAgent, version)
}

// ResolveWardenStemcell resolves a Warden (bosh-lite) stemcell by OS name from bosh.io
// It tries without the -go_agent suffix first (for newer stemcells like Noble),
// and falls back to with the suffix (for older stemcells like Jammy)
func (c *Client) ResolveWardenStemcell(ctx context.Context, osName, version string) (*StemcellInfo, error) {
	return c.ResolveStemcellByInfra(ctx, "warden-boshlite", osName, version)
}

// ResolveOpenStackStemcell resolves an OpenStack stemcell by OS name from bosh.io
// It tries without the -go_agent suffix first (for newer stemcells like Noble),
// and falls back to with the suffix (for older stemcells like Jammy)
func (c *Client) ResolveOpenStackStemcell(ctx context.Context, osName, version string) (*StemcellInfo, error) {
	return c.ResolveStemcellByInfra(ctx, "openstack-kvm", osName, version)
}

// OpenStackStemcellName converts an OS name to the OpenStack stemcell name
// e.g., "ubuntu-jammy" -> "bosh-openstack-kvm-ubuntu-jammy-go_agent"
func OpenStackStemcellName(os string) string {
	return fmt.Sprintf("bosh-openstack-kvm-%s-go_agent", os)
}

// IncusStemcellName returns the BOSH stemcell name for Incus CPI
// This matches the name used in the stemcell manifest
// e.g., "ubuntu-jammy" -> "bosh-openstack-kvm-ubuntu-jammy-go_agent"
func IncusStemcellName(os string) string {
	// Incus CPI uses the same stemcell format as OpenStack
	return OpenStackStemcellName(os)
}
