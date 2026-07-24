package cmdsafelist

import "testing"

func TestResolveKnown(t *testing.T) {
	if _, err := Resolve("qemu-img"); err != nil {
		t.Errorf("qemu-img should resolve: %v", err)
	}
	if _, err := Resolve("sha256sum"); err != nil {
		t.Errorf("sha256sum should resolve: %v", err)
	}
}

func TestResolveUnknown(t *testing.T) {
	if _, err := Resolve("rm"); err == nil {
		t.Error("rm must not be allowed")
	}
	if _, err := Resolve("bash"); err == nil {
		t.Error("bash must not be allowed")
	}
}

func TestAllowed(t *testing.T) {
	if !Allowed("qemu-img") {
		t.Error("qemu-img should be allowed")
	}
	if Allowed("curl") {
		t.Error("curl must not be allowed")
	}
}