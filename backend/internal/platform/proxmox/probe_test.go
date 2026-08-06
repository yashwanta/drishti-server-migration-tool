package proxmox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drishti/hypershift/internal/domain"
)

func TestProbeAuthenticatesAndReadsVersion(t *testing.T) {
	const wantAuth = "PVEAPIToken=user@pve!drishti=topsecret"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != wantAuth {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"version":"9.1.1","release":"9.1"}}`))
	}))
	defer srv.Close()
	t.Setenv("DRISHTI_SECRET_LAB_TOKEN_ID", "user@pve!drishti")
	t.Setenv("DRISHTI_SECRET_LAB_TOKEN_SECRET", "topsecret")
	msg, err := NewProbe().Test(context.Background(), domain.Connection{
		Kind: domain.PlatformProxmox, Endpoint: srv.URL + "/#ui",
		SecretRef: "lab", InsecureTLS: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg != "Authenticated to Proxmox VE 9.1.1 (release 9.1)." {
		t.Fatalf("message = %q", msg)
	}
}

func TestProbeRequiresCredentials(t *testing.T) {
	_, err := NewProbe().Test(context.Background(), domain.Connection{
		Kind: domain.PlatformProxmox, Endpoint: "https://pve:8006", SecretRef: "missing",
	})
	if err == nil {
		t.Fatal("expected missing credentials error")
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	got, err := normalizeEndpoint("192.168.1.135:8006/#v1:0")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://192.168.1.135:8006" {
		t.Fatalf("endpoint = %q", got)
	}
}
