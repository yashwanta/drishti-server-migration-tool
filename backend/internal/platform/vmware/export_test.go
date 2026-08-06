package vmware

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixtureDownloader map[string][]byte

func (f fixtureDownloader) Download(_ context.Context, remotePath, localPath string) error {
	data, ok := f[remotePath]
	if !ok {
		return fmt.Errorf("missing fixture %s", remotePath)
	}
	return os.WriteFile(localPath, data, 0o600)
}

func TestExportVMDKDownloadsDescriptorAndExtent(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "disk.vmdk")
	descriptor := []byte("# Disk DescriptorFile\nversion=1\nRW 8 VMFS \"disk-flat.vmdk\"\n")
	downloader := fixtureDownloader{
		"vm/disk.vmdk":      descriptor,
		"vm/disk-flat.vmdk": []byte("eight sectors of fixture data"),
	}
	result, manifest, err := exportVMDK(context.Background(), downloader, "vm/disk.vmdk", destination)
	if err != nil {
		t.Fatalf("exportVMDK: %v", err)
	}
	if result.Path != destination || result.SHA256 == "" || result.SizeBytes <= int64(len(descriptor)) {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(manifest.Files) != 2 || manifest.Descriptor != "disk.vmdk" {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(destination), "disk-flat.vmdk")); err != nil {
		t.Fatalf("extent missing: %v", err)
	}
}

func TestExportVMDKRejectsUnsafeExtent(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "disk.vmdk")
	downloader := fixtureDownloader{
		"vm/disk.vmdk": []byte("RW 8 VMFS \"../outside-flat.vmdk\"\n"),
	}
	if _, _, err := exportVMDK(context.Background(), downloader, "vm/disk.vmdk", destination); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("expected unsafe extent error, got %v", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination should not exist after refusal, stat err=%v", err)
	}
}

func TestExportVMDKRejectsMissingAndDuplicateExtents(t *testing.T) {
	for name, descriptor := range map[string]string{
		"missing":   "# no extents\n",
		"duplicate": "RW 8 VMFS \"disk-flat.vmdk\"\nRW 8 VMFS \"disk-flat.vmdk\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "disk.vmdk")
			downloader := fixtureDownloader{"vm/disk.vmdk": []byte(descriptor)}
			if _, _, err := exportVMDK(context.Background(), downloader, "vm/disk.vmdk", destination); err == nil {
				t.Fatal("expected descriptor to be rejected")
			}
		})
	}
}

func TestExportVMDKDoesNotOverwriteExtent(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "disk.vmdk")
	extent := filepath.Join(dir, "disk-flat.vmdk")
	if err := os.WriteFile(extent, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	downloader := fixtureDownloader{"vm/disk.vmdk": []byte("RW 8 VMFS \"disk-flat.vmdk\"\n")}
	if _, _, err := exportVMDK(context.Background(), downloader, "vm/disk.vmdk", destination); err == nil {
		t.Fatal("expected existing extent to be rejected")
	}
	data, _ := os.ReadFile(extent)
	if string(data) != "keep" {
		t.Fatalf("existing extent was changed: %q", data)
	}
}
