package proxmox

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
)

type endpointResolver func(context.Context, string) ([]net.IPAddr, error)

func defaultEndpointResolver(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}

// rejectResolvedDenylistAliases prevents a protected address from being
// reached through a different hostname. Resolution failure is fail-closed.
func rejectResolvedDenylistAliases(ctx context.Context, endpoint string, denylist []string, resolve endpointResolver) error {
	if len(denylist) == 0 {
		return nil
	}
	host, err := endpointHostname(endpoint)
	if err != nil {
		return err
	}
	endpointIPs, err := resolveHost(ctx, host, resolve)
	if err != nil {
		return fmt.Errorf("resolve target endpoint for denylist enforcement: %w", err)
	}
	for _, denied := range denylist {
		deniedHost, err := endpointHostname(denied)
		if err != nil {
			return fmt.Errorf("invalid mutation denylist entry: %w", err)
		}
		deniedIPs, err := resolveHost(ctx, deniedHost, resolve)
		if err != nil {
			return fmt.Errorf("resolve mutation denylist entry: %w", err)
		}
		for _, endpointIP := range endpointIPs {
			for _, deniedIP := range deniedIPs {
				if endpointIP.Equal(deniedIP) {
					return fmt.Errorf("platform mutation endpoint resolves to a denylisted address")
				}
			}
		}
	}
	return nil
}

func endpointHostname(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || u.User != nil {
		return "", fmt.Errorf("invalid platform endpoint")
	}
	return strings.TrimSuffix(strings.ToLower(u.Hostname()), "."), nil
}

func resolveHost(ctx context.Context, host string, resolve endpointResolver) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	addresses, err := resolve(ctx, host)
	if err != nil || len(addresses) == 0 {
		if err == nil {
			err = fmt.Errorf("hostname returned no addresses")
		}
		return nil, err
	}
	out := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		if address.IP != nil {
			out = append(out, address.IP)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("hostname returned no usable addresses")
	}
	return out, nil
}
