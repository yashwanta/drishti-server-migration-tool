package safety

import (
	"testing"

	"github.com/drishti/hypershift/internal/config"
)

func TestMutationPolicy(t *testing.T) {
	tests := []struct {
		name     string
		policy   MutationPolicy
		endpoint string
		wantErr  bool
	}{
		{"lab enabled", MutationPolicy{Mode: config.ModeLab, Enabled: true}, "https://lab-vcenter:443/sdk", false},
		{"live enabled", MutationPolicy{Mode: config.ModeLive, Enabled: true}, "lab-vcenter", false},
		{"mock rejected", MutationPolicy{Mode: config.ModeMock, Enabled: true}, "lab-vcenter", true},
		{"production rejected", MutationPolicy{Mode: config.ModeProduction, Enabled: true}, "lab-vcenter", true},
		{"flag required", MutationPolicy{Mode: config.ModeLab}, "lab-vcenter", true},
		{"denylisted host", MutationPolicy{Mode: config.ModeLab, Enabled: true, Denylist: []string{"PROD.example"}}, "https://prod.example/sdk", true},
		{"denylisted host port", MutationPolicy{Mode: config.ModeLab, Enabled: true, Denylist: []string{"10.0.0.2:443"}}, "https://10.0.0.2:443/sdk", true},
		{"credentials rejected", MutationPolicy{Mode: config.ModeLab, Enabled: true}, "https://user:pass@lab/sdk", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Authorize(tt.endpoint)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Authorize() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
