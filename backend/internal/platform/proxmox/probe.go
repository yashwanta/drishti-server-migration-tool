// Package proxmox provides narrowly scoped, read-only Proxmox VE API access.
package proxmox

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/drishti/hypershift/internal/domain"
)

var secretKeyRE = regexp.MustCompile(`[^A-Z0-9]+`)
var apiTokenRE = regexp.MustCompile(`PVEAPIToken=[^,\s\"']+`)

// Probe performs an authenticated, read-only request to the PVE version API.
// Credentials are resolved from environment variables derived from SecretRef.
type Probe struct{}

func NewProbe() *Probe { return &Probe{} }

func (p *Probe) Test(ctx context.Context, conn domain.Connection) (string, error) {
	if conn.Kind != domain.PlatformProxmox {
		return "", fmt.Errorf("real connectivity is not implemented for platform %q", conn.Kind)
	}
	client, err := newAPIClient(conn)
	if err != nil {
		return "", err
	}
	var body struct {
		Version string `json:"version"`
		Release string `json:"release"`
	}
	if err := client.get(ctx, "/api2/json/version", &body); err != nil {
		return "", err
	}
	if body.Version == "" {
		return "", fmt.Errorf("Proxmox API response did not include a version")
	}
	return fmt.Sprintf("Authenticated to Proxmox VE %s (release %s).", body.Version, body.Release), nil
}

type apiClient struct {
	baseURL string
	auth    string
	http    *http.Client
}

func newAPIClient(conn domain.Connection) (*apiClient, error) {
	endpoint, err := normalizeEndpoint(conn.Endpoint)
	if err != nil {
		return nil, err
	}
	tokenID, tokenSecret, err := credentials(conn.SecretRef)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: conn.InsecureTLS} // #nosec G402 -- explicit lab-only setting
	return &apiClient{
		baseURL: endpoint,
		auth:    "PVEAPIToken=" + tokenID + "=" + tokenSecret,
		http:    &http.Client{Transport: transport, Timeout: 15 * time.Second},
	}, nil
}

func (c *apiClient) get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

func (c *apiClient) postForm(ctx context.Context, path string, values url.Values, out any) error {
	return c.do(ctx, http.MethodPost, path, strings.NewReader(values.Encode()), out)
}

func (c *apiClient) do(ctx context.Context, method, path string, body *strings.Reader, out any) error {
	var requestBody io.Reader
	if body != nil {
		requestBody = body
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, requestBody)
	if err != nil {
		return fmt.Errorf("build Proxmox request: %w", err)
	}
	req.Header.Set("Authorization", c.auth)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("connect to Proxmox endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		detail := strings.TrimSpace(apiTokenRE.ReplaceAllString(string(message), "PVEAPIToken=[REDACTED]"))
		if detail == "" {
			return fmt.Errorf("Proxmox API %s returned HTTP %d", path, resp.StatusCode)
		}
		return fmt.Errorf("Proxmox API %s returned HTTP %d: %s", path, resp.StatusCode, detail)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("decode Proxmox response: %w", err)
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("decode Proxmox data for %s: %w", path, err)
	}
	return nil
}

func normalizeEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("Proxmox endpoint is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid Proxmox endpoint")
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("Proxmox endpoint must use https")
	}
	// Browser fragments and UI paths are never part of the API endpoint.
	u.Fragment, u.RawQuery, u.Path = "", "", ""
	return strings.TrimRight(u.String(), "/"), nil
}

func credentials(secretRef string) (string, string, error) {
	key := strings.Trim(secretKeyRE.ReplaceAllString(strings.ToUpper(secretRef), "_"), "_")
	if key == "" {
		return "", "", fmt.Errorf("secret reference is required")
	}
	prefix := "DRISHTI_SECRET_" + key
	id := strings.TrimSpace(os.Getenv(prefix + "_TOKEN_ID"))
	secret := strings.TrimSpace(os.Getenv(prefix + "_TOKEN_SECRET"))
	if id == "" || secret == "" {
		return "", "", fmt.Errorf("credentials not configured for secret reference %q", secretRef)
	}
	if !strings.Contains(id, "!") {
		return "", "", fmt.Errorf("token ID for secret reference %q must use user@realm!token format", secretRef)
	}
	return id, secret, nil
}
