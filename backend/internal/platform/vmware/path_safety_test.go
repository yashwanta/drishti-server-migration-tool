package vmware

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateExportPath(t *testing.T) {
	workspace := localTempDir(t)
	valid := filepath.Join(workspace, "job-1", "disk.vmdk")
	got, err := validateExportPath(workspace, valid)
	if err != nil {
		t.Fatalf("valid path rejected: %v", err)
	}
	if got != valid {
		t.Fatalf("path = %q, want %q", got, valid)
	}
	if _, err := validateExportPath(workspace, filepath.Join(filepath.Dir(workspace), "escape.vmdk")); err == nil {
		t.Fatal("expected workspace escape to be rejected")
	}
	if _, err := validateExportPath(workspace, filepath.Join(workspace, "disk.raw")); err == nil {
		t.Fatal("expected non-VMDK extension to be rejected")
	}
	if err := os.WriteFile(valid, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateExportPath(workspace, valid); err == nil {
		t.Fatal("expected existing export to be rejected")
	}
}

func localTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", ".item0-test-")
	if err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(abs) })
	return abs
}

func TestValidateExportPathRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks can require elevated Windows privileges")
	}
	workspace := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(workspace, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := validateExportPath(workspace, filepath.Join(link, "disk.vmdk")); err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
}
