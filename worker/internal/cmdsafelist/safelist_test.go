package cmdsafelist

import "testing"

func TestResolveKnown(t *testing.T) {
	if _, err := Resolve("qemu-img"); err != nil {
		t.Errorf("qemu-img should resolve: %v", err)
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

func TestValidateArgsAllowsOnlyTypedQEMUConversion(t *testing.T) {
	valid := []string{"convert", "-f", "vmdk", "-O", "qcow2", "/workspace/input.vmdk", "/workspace/output.partial"}
	if err := ValidateArgs("qemu-img", valid); err != nil {
		t.Fatalf("valid conversion rejected: %v", err)
	}
	invalid := [][]string{
		{"info", "/etc/passwd"},
		{"convert", "-f", "raw", "-O", "qcow2", "/in", "/out"},
		{"convert", "-f", "vmdk", "-O", "vmdk", "/in", "/out"},
		{"convert", "-f", "vmdk", "-O", "raw", "--image-opts", "/out"},
	}
	for _, args := range invalid {
		if err := ValidateArgs("qemu-img", args); err == nil {
			t.Errorf("unsafe arguments accepted: %#v", args)
		}
	}
}
