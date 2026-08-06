package proxmox

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/drishti/hypershift/internal/config"
	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/platform"
	"github.com/drishti/hypershift/internal/safety"
)

func TestImportDiskUsesSharedRootAndIsIdempotent(t *testing.T) {
	var mu sync.Mutex
	imported := false
	importCalls := 0
	owner := ownershipMarker("plan-create")
	spec := platform.AttachDiskSpec{Path: "/mnt/drishti-import/job-1/disk.raw", Format: domain.DiskRaw, SizeBytes: 4096, Boot: true, Controller: "virtio-scsi", StorageID: "local-lvm", DeviceIndex: 0, IdempotencyKey: "plan-create:disk-0"}
	diskMarker := diskImportMarker(spec, "scsi0", spec.Path)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		assertTargetAuth(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/resources":
			writeTargetEnvelope(t, w, `[{"type":"qemu","node":"pve","vmid":420,"status":"stopped"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/qemu/420/status/current":
			writeTargetEnvelope(t, w, `{"status":"stopped"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/storage":
			writeTargetEnvelope(t, w, `[{"storage":"local-lvm","content":"images","active":1,"enabled":1,"avail":1073741824}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/qemu/420/config":
			description := owner
			disk := ""
			boot := ""
			if imported {
				description += "\n" + diskMarker
				disk = `,"scsi0":"local-lvm:vm-420-disk-0,discard=on"`
				boot = `,"boot":"order=scsi0"`
			}
			writeTargetEnvelope(t, w, fmt.Sprintf(`{"description":%q%s%s}`, description, disk, boot))
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/nodes/pve/qemu/420/config":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if got := r.Form.Get("scsi0"); got != "local-lvm:0,import-from=/mnt/drishti-import/job-1/disk.raw,format=raw" {
				t.Fatalf("import value = %q", got)
			}
			if r.Form.Get("boot") != "order=scsi0" || !strings.Contains(r.Form.Get("description"), diskMarker) {
				t.Fatalf("import form lacks boot/idempotency marker: %v", r.Form)
			}
			importCalls++
			imported = true
			writeTargetEnvelope(t, w, `"UPID:import"`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/tasks/"):
			writeTargetEnvelope(t, w, `{"status":"stopped","exitstatus":"OK"}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()
	target := newFixtureTarget(t, srv.URL, safety.MutationPolicy{Mode: config.ModeLab, Enabled: true})
	if err := target.ImportDisk(context.Background(), 420, spec); err != nil {
		t.Fatalf("ImportDisk: %v", err)
	}
	if err := target.ImportDisk(context.Background(), 420, spec); err != nil {
		t.Fatalf("idempotent ImportDisk: %v", err)
	}
	if importCalls != 1 {
		t.Fatalf("import calls = %d, want 1", importCalls)
	}
}

func TestImportDiskRejectsUnsafePathAndUnsupportedSpecWithoutHTTP(t *testing.T) {
	requests := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer srv.Close()
	target := newFixtureTarget(t, srv.URL, safety.MutationPolicy{Mode: config.ModeLab, Enabled: true})
	base := platform.AttachDiskSpec{Path: "/mnt/drishti-import/job/disk.raw", Format: domain.DiskRaw, SizeBytes: 4096, Controller: "virtio-scsi", StorageID: "local-lvm", IdempotencyKey: "disk"}
	tests := []platform.AttachDiskSpec{
		func() platform.AttachDiskSpec { v := base; v.Path = "/mnt/other/disk.raw"; return v }(),
		func() platform.AttachDiskSpec { v := base; v.Path = "/mnt/drishti-import/../escape.raw"; return v }(),
		func() platform.AttachDiskSpec { v := base; v.Path = "/mnt/drishti-import/job/a,b.raw"; return v }(),
		func() platform.AttachDiskSpec { v := base; v.Format = domain.DiskVMDK; return v }(),
		func() platform.AttachDiskSpec { v := base; v.Controller = "sata"; return v }(),
		func() platform.AttachDiskSpec { v := base; v.DeviceIndex = 31; return v }(),
		func() platform.AttachDiskSpec { v := base; v.IdempotencyKey = ""; return v }(),
	}
	for i, spec := range tests {
		if err := target.ImportDisk(context.Background(), 420, spec); err == nil {
			t.Fatalf("case %d: expected refusal", i)
		}
	}
	if requests != 0 {
		t.Fatalf("invalid imports made %d HTTP requests", requests)
	}
}

func TestImportDiskRejectsOccupiedUnmarkedSlot(t *testing.T) {
	postCalls := 0
	owner := ownershipMarker("plan-create")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTargetAuth(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/resources":
			writeTargetEnvelope(t, w, `[{"type":"qemu","node":"pve","vmid":420,"status":"stopped"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/qemu/420/status/current":
			writeTargetEnvelope(t, w, `{"status":"stopped"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/storage":
			writeTargetEnvelope(t, w, `[{"storage":"local-lvm","content":"images","active":1,"enabled":1,"avail":1073741824}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/qemu/420/config":
			writeTargetEnvelope(t, w, fmt.Sprintf(`{"description":%q,"scsi0":"local-lvm:vm-420-disk-foreign"}`, owner))
		case r.Method == http.MethodPost:
			postCalls++
			writeTargetEnvelope(t, w, `null`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()
	target := newFixtureTarget(t, srv.URL, safety.MutationPolicy{Mode: config.ModeLab, Enabled: true})
	spec := platform.AttachDiskSpec{Path: "/mnt/drishti-import/job/disk.raw", Format: domain.DiskRaw, SizeBytes: 4096, Controller: "virtio-scsi", StorageID: "local-lvm", DeviceIndex: 0, IdempotencyKey: "disk"}
	if err := target.ImportDisk(context.Background(), 420, spec); err == nil || !strings.Contains(err.Error(), "occupied") {
		t.Fatalf("expected occupied-slot refusal, got %v", err)
	}
	if postCalls != 0 {
		t.Fatalf("occupied slot caused %d mutation calls", postCalls)
	}
}

func TestImportDiskRejectsInsufficientStorageBeforeMutation(t *testing.T) {
	postCalls := 0
	owner := ownershipMarker("plan-create")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertTargetAuth(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/resources":
			writeTargetEnvelope(t, w, `[{"type":"qemu","node":"pve","vmid":420,"status":"stopped"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/qemu/420/status/current":
			writeTargetEnvelope(t, w, `{"status":"stopped"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/qemu/420/config":
			writeTargetEnvelope(t, w, fmt.Sprintf(`{"description":%q}`, owner))
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve/storage":
			writeTargetEnvelope(t, w, `[{"storage":"local-lvm","content":"images","active":1,"enabled":1,"avail":1024}]`)
		case r.Method == http.MethodPost:
			postCalls++
			writeTargetEnvelope(t, w, `null`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()
	target := newFixtureTarget(t, srv.URL, safety.MutationPolicy{Mode: config.ModeLab, Enabled: true})
	spec := platform.AttachDiskSpec{Path: "/mnt/drishti-import/job/disk.raw", Format: domain.DiskRaw, SizeBytes: 4096, Controller: "virtio-scsi", StorageID: "local-lvm", DeviceIndex: 0, IdempotencyKey: "disk"}
	if err := target.ImportDisk(context.Background(), 420, spec); err == nil || !strings.Contains(err.Error(), "available") {
		t.Fatalf("expected capacity refusal, got %v", err)
	}
	if postCalls != 0 {
		t.Fatalf("capacity refusal caused %d mutation calls", postCalls)
	}
}

func TestNormalizeImportRootAndPath(t *testing.T) {
	root, err := normalizeImportRoot("/mnt/drishti-import/")
	if err != nil || root != "/mnt/drishti-import" {
		t.Fatalf("root=%q err=%v", root, err)
	}
	if got, err := normalizeImportPath(root, "/mnt/drishti-import/job/disk.qcow2"); err != nil || got == "" {
		t.Fatalf("path=%q err=%v", got, err)
	}
	for _, bad := range []string{"", "/", "relative", "/mnt/drishti-import", "/mnt/drishti-import/../escape"} {
		if _, err := normalizeImportPath(root, bad); err == nil {
			t.Fatalf("unsafe path %q accepted", bad)
		}
	}
}
