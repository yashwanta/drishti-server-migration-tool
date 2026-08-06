package proxmox

import (
	"context"
	"fmt"
	"net"
	"testing"
)

func TestRejectResolvedDenylistAliases(t *testing.T) {
	resolver := func(_ context.Context, host string) ([]net.IPAddr, error) {
		addresses := map[string]string{
			"lab-alias.example": "10.0.0.50",
			"prod-pve.example":  "10.0.0.50",
			"safe-pve.example":  "10.0.0.60",
		}
		value, ok := addresses[host]
		if !ok {
			return nil, fmt.Errorf("not found")
		}
		return []net.IPAddr{{IP: net.ParseIP(value)}}, nil
	}
	if err := rejectResolvedDenylistAliases(context.Background(), "https://lab-alias.example:8006", []string{"prod-pve.example"}, resolver); err == nil {
		t.Fatal("expected DNS alias of denylisted endpoint to be rejected")
	}
	if err := rejectResolvedDenylistAliases(context.Background(), "https://safe-pve.example:8006", []string{"prod-pve.example"}, resolver); err != nil {
		t.Fatalf("safe endpoint rejected: %v", err)
	}
}

func TestRejectResolvedDenylistFailsClosed(t *testing.T) {
	resolver := func(_ context.Context, host string) ([]net.IPAddr, error) {
		return nil, fmt.Errorf("DNS unavailable for %s", host)
	}
	if err := rejectResolvedDenylistAliases(context.Background(), "https://unknown.example", []string{"10.0.0.50"}, resolver); err == nil {
		t.Fatal("expected DNS failure to reject mutation")
	}
}
