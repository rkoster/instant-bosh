package configserver

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// Client provides access to the config-server API
type Client struct {
	serverURL    string
	uaaURL       string
	clientID     string
	clientSecret string
	httpClient   *http.Client

	// Token caching
	tokenMu     sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

// Credential represents a credential stored in config-server
type Credential struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Value interface{} `json:"value"`
}

// ErrNotConfigured is returned when required environment variables are not set
type ErrNotConfigured struct {
	MissingVars []string
}

func (e *ErrNotConfigured) Error() string {
	return fmt.Sprintf("config-server environment not configured. Missing: %s\nRun: eval \"$(ibosh docker print-env)\" or eval \"$(ibosh incus print-env)\"",
		strings.Join(e.MissingVars, ", "))
}

// NewClientFromEnv creates a new config-server client from environment variables
func NewClientFromEnv() (*Client, error) {
	serverURL := os.Getenv("CONFIG_SERVER_URL")
	clientID := os.Getenv("CONFIG_SERVER_CLIENT")
	clientSecret := os.Getenv("CONFIG_SERVER_SECRET")
	caCert := os.Getenv("CONFIG_SERVER_CA_CERT")
	uaaURL := os.Getenv("UAA_URL")
	uaaCACert := os.Getenv("UAA_CA_CERT")

	var missing []string
	if serverURL == "" {
		missing = append(missing, "CONFIG_SERVER_URL")
	}
	if clientID == "" {
		missing = append(missing, "CONFIG_SERVER_CLIENT")
	}
	if clientSecret == "" {
		missing = append(missing, "CONFIG_SERVER_SECRET")
	}
	if caCert == "" {
		missing = append(missing, "CONFIG_SERVER_CA_CERT")
	}
	if uaaURL == "" {
		missing = append(missing, "UAA_URL")
	}
	if uaaCACert == "" {
		missing = append(missing, "UAA_CA_CERT")
	}

	if len(missing) > 0 {
		return nil, &ErrNotConfigured{MissingVars: missing}
	}

	return NewClient(serverURL, uaaURL, clientID, clientSecret, caCert, uaaCACert)
}

// NewClient creates a new config-server client
func NewClient(serverURL, uaaURL, clientID, clientSecret, caCert, uaaCACert string) (*Client, error) {
	// Create TLS config with both CA certs
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM([]byte(caCert)) {
		return nil, fmt.Errorf("failed to parse config-server CA certificate")
	}
	if uaaCACert != caCert {
		if !caCertPool.AppendCertsFromPEM([]byte(uaaCACert)) {
			return nil, fmt.Errorf("failed to parse UAA CA certificate")
		}
	}

	tlsConfig := &tls.Config{
		RootCAs:    caCertPool,
		MinVersion: tls.VersionTLS12,
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	return &Client{
		serverURL:    strings.TrimSuffix(serverURL, "/"),
		uaaURL:       strings.TrimSuffix(uaaURL, "/"),
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   httpClient,
	}, nil
}

// getAccessToken retrieves or refreshes the OAuth2 access token from UAA
func (c *Client) getAccessToken() (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	// Return cached token if still valid (with 30 second buffer)
	if c.accessToken != "" && time.Now().Add(30*time.Second).Before(c.tokenExpiry) {
		return c.accessToken, nil
	}

	// Request new token from UAA
	tokenURL := c.uaaURL + "/oauth/token"

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", c.clientID)
	data.Set("client_secret", c.clientSecret)

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to request token from UAA: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("UAA token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	c.accessToken = tokenResp.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return c.accessToken, nil
}

// Get retrieves a credential by name
func (c *Client) Get(name string) (*Credential, error) {
	// Ensure name starts with /
	if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}

	// Get access token
	token, err := c.getAccessToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	// Build URL: /v1/data?name=<name>
	u, err := url.Parse(c.serverURL + "/v1/data")
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	q := u.Query()
	q.Set("name", name)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("credential not found: %s", name)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	// Config-server returns {"data": [...]} wrapper
	var wrapper struct {
		Data []Credential `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(wrapper.Data) == 0 {
		return nil, fmt.Errorf("credential not found: %s", name)
	}

	return &wrapper.Data[0], nil
}

// ErrFindNotSupported is returned when Find is called but config-server doesn't support listing
var ErrFindNotSupported = fmt.Errorf("config-server does not support listing credentials. Use 'bosh variables -d <deployment>' to see available credential names, then use 'ibosh creds get <name>'")

// Find is not supported by config-server (unlike CredHub)
// Config-server only supports getting credentials by exact name
func (c *Client) Find(pathPrefix string) ([]Credential, error) {
	return nil, ErrFindNotSupported
}

// FormatValue formats a credential value for display
func FormatValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case map[string]interface{}:
		// For complex types like certificates, format as YAML-like output
		var lines []string
		for key, val := range v {
			switch valTyped := val.(type) {
			case string:
				// Multi-line strings get special formatting
				if strings.Contains(valTyped, "\n") {
					lines = append(lines, fmt.Sprintf("%s: |", key))
					for _, line := range strings.Split(valTyped, "\n") {
						lines = append(lines, fmt.Sprintf("  %s", line))
					}
				} else {
					lines = append(lines, fmt.Sprintf("%s: %s", key, valTyped))
				}
			default:
				lines = append(lines, fmt.Sprintf("%s: %v", key, val))
			}
		}
		return strings.Join(lines, "\n")
	default:
		// For other types, use JSON
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

// FormatValueJSON formats a credential value as JSON
func FormatValueJSON(value interface{}) string {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(b)
}
