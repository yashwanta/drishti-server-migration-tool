// Package proxmox provides narrowly scoped, read-only Proxmox VE API access.
package proxmox

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/drishti/hypershift/internal/domain"
)

var secretKeyRE = regexp.MustCompile(`[^A-Z0-9]+`)

// Probe performs an authenticated, read-only request to the PVE version API.
// Credentials are resolved from environment variables derived from SecretRef.
type Probe struct{}

func NewProbe() *Probe { return &Probe{} }

func (p *Probe) Test(ctx context.Context, conn domain.Connection) (string, error) {
	if conn.Kind != domain.PlatformProxmox {
		return "", fmt.Errorf("real connectivity is not implemented for platform %q", conn.Kind)
	}
	endpoint, err := normalizeEndpoint(conn.Endpoint)
	if err != nil {
		return "", err
	}
	tokenID, tokenSecret, err := credentials(conn.SecretRef)
	if err != nil {
		return "", err
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: conn.InsecureTLS} // #nosec G402 -- explicit lab-only setting
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/api2/json/version", nil)
	if err != nil {
		return "", fmt.Errorf("build Proxmox request: %w", err)
	}
	req.Header.Set("Authorization", "PVEAPIToken="+tokenID+"="+tokenSecret)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("connect to Proxmox endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Proxmox API returned HTTP %d", resp.StatusCode)
	}
	var body struct {
		Data struct {
			Version string `json:"version"`
			Release string `json:"release"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decode Proxmox response: %w", err)
	}
	if body.Data.Version == "" {
		return "", fmt.Errorf("Proxmox API response did not include a version")
	}
	return fmt.Sprintf("Authenticated to Proxmox VE %s (release %s).", body.Data.Version, body.Data.Release), nil
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
