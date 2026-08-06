// Package safety contains fail-closed interlocks shared by real platform
// adapters. It performs no network access.
package safety

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/drishti/hypershift/internal/config"
)

// MutationPolicy authorizes only explicitly enabled lab/live mutations.
// Production is deliberately prohibited in this phase.
type MutationPolicy struct {
	Mode     config.RunMode
	Enabled  bool
	Denylist []string
}

// Authorize rejects a mutation unless both the runtime tier and explicit flag
// permit it and the endpoint is not protected.
func (p MutationPolicy) Authorize(endpoint string) error {
	if p.Mode != config.ModeLab && p.Mode != config.ModeLive {
		return fmt.Errorf("platform mutation is prohibited in %s mode", p.Mode)
	}
	if !p.Enabled {
		return fmt.Errorf("platform mutation requires DRISHTI_ENABLE_PLATFORM_MUTATION=true")
	}
	host, port, err := endpointIdentity(endpoint)
	if err != nil {
		return err
	}
	for _, denied := range p.Denylist {
		dh, dp, err := endpointIdentity(denied)
		if err != nil {
			return fmt.Errorf("invalid mutation denylist entry: %w", err)
		}
		if strings.EqualFold(host, dh) && (dp == "" || port == dp) {
			return fmt.Errorf("platform mutation endpoint is denylisted")
		}
	}
	return nil
}

func endpointIdentity(raw string) (string, string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", "", fmt.Errorf("empty platform endpoint")
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || u.User != nil {
		return "", "", fmt.Errorf("invalid platform endpoint")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	}
	return host, u.Port(), nil
}
