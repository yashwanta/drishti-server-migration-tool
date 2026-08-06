package proxmox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/platform"
)

const diskMarkerPrefix = "drishti:disk:"

// AttachDisk satisfies platform.TargetAdapter and delegates to ImportDisk.
func (t *Target) AttachDisk(ctx context.Context, vmid int, spec platform.AttachDiskSpec) error {
	return t.ImportDisk(ctx, vmid, spec)
}

// ImportDisk asks Proxmox to import a worker-produced image from the configured
// target-visible shared import root and attach it to a deterministic SCSI slot.
func (t *Target) ImportDisk(ctx context.Context, vmid int, spec platform.AttachDiskSpec) error {
	if err := t.authorizeMutation(ctx); err != nil {
		return err
	}
	if err := validateImportSpec(spec); err != nil {
		return err
	}
	importPath, err := normalizeImportPath(t.importRoot, spec.Path)
	if err != nil {
		return err
	}
	node, err := t.ownedVMNode(ctx, vmid)
	if err != nil {
		return err
	}
	if err := t.requireStopped(ctx, node, vmid); err != nil {
		return fmt.Errorf("disk import requires a stopped target VM: %w", err)
	}
	if err := t.requireImageStorage(ctx, node, spec.StorageID, spec.SizeBytes); err != nil {
		return err
	}
	cfg, _, err := t.readVMConfig(ctx, node, vmid)
	if err != nil {
		return err
	}
	slot := "scsi" + strconv.Itoa(spec.DeviceIndex)
	marker := diskImportMarker(spec, slot, importPath)
	description := rawString(cfg["description"])
	if strings.Contains(description, marker) {
		if rawString(cfg[slot]) == "" {
			return fmt.Errorf("disk import marker exists but target slot %s is empty", slot)
		}
		return nil
	}
	if rawString(cfg[slot]) != "" {
		return fmt.Errorf("target disk slot %s is already occupied without the matching idempotency marker", slot)
	}

	updatedDescription := strings.TrimSpace(description)
	if updatedDescription != "" {
		updatedDescription += "\n"
	}
	updatedDescription += marker
	diskValue := spec.StorageID + ":0,import-from=" + importPath + ",format=" + string(spec.Format)
	values := url.Values{
		slot:          {diskValue},
		"description": {updatedDescription},
		"scsihw":      {"virtio-scsi-single"},
	}
	if spec.Boot {
		values.Set("boot", "order="+slot)
	}
	configPath := fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/config", url.PathEscape(node), vmid)
	var response json.RawMessage
	if err := t.client.postForm(ctx, configPath, values, &response); err != nil {
		return fmt.Errorf("import target disk into %s: %w", slot, err)
	}
	if upid := rawString(response); strings.HasPrefix(upid, "UPID:") {
		if err := t.waitTask(ctx, node, upid); err != nil {
			return fmt.Errorf("import target disk into %s: %w", slot, err)
		}
	}
	verified, _, err := t.readVMConfig(ctx, node, vmid)
	if err != nil {
		return err
	}
	attached := rawString(verified[slot])
	if attached == "" || strings.Contains(attached, "import-from=") {
		return fmt.Errorf("Proxmox did not finalize imported disk in target slot %s", slot)
	}
	if !strings.Contains(rawString(verified["description"]), marker) {
		return fmt.Errorf("target disk import idempotency marker was not retained")
	}
	if spec.Boot && !strings.Contains(rawString(verified["boot"]), slot) {
		return fmt.Errorf("target boot order does not include imported disk %s", slot)
	}
	return nil
}

func validateImportSpec(spec platform.AttachDiskSpec) error {
	if spec.Format != domain.DiskRaw && spec.Format != domain.DiskQCOW2 {
		return fmt.Errorf("target disk format must be raw or qcow2")
	}
	if spec.SizeBytes <= 0 {
		return fmt.Errorf("target disk size must be positive")
	}
	if spec.Controller != "virtio-scsi" {
		return fmt.Errorf("only the virtio-scsi target controller is supported")
	}
	if err := validatePVEID("storage", spec.StorageID); err != nil {
		return err
	}
	if spec.DeviceIndex < 0 || spec.DeviceIndex > 30 {
		return fmt.Errorf("target SCSI device index must be between 0 and 30")
	}
	if spec.IdempotencyKey == "" {
		return fmt.Errorf("target disk import requires an idempotency key")
	}
	return nil
}

func normalizeImportRoot(root string) (string, error) {
	value := strings.TrimSpace(root)
	if value == "" || !path.IsAbs(value) {
		return "", fmt.Errorf("DRISHTI_PROXMOX_IMPORT_ROOT must be an absolute POSIX path")
	}
	if strings.ContainsAny(value, ",\x00\r\n") {
		return "", fmt.Errorf("Proxmox import root contains unsafe characters")
	}
	cleaned := path.Clean(value)
	if cleaned == "/" {
		return "", fmt.Errorf("Proxmox import root cannot be filesystem root")
	}
	return cleaned, nil
}

func normalizeImportPath(root, candidate string) (string, error) {
	value := strings.TrimSpace(candidate)
	if value == "" || !path.IsAbs(value) || strings.ContainsAny(value, ",\x00\r\n") {
		return "", fmt.Errorf("target import path must be an absolute, safe POSIX path")
	}
	cleaned := path.Clean(value)
	if cleaned == root || !strings.HasPrefix(cleaned, root+"/") {
		return "", fmt.Errorf("target import path must be beneath DRISHTI_PROXMOX_IMPORT_ROOT")
	}
	return cleaned, nil
}

func diskImportMarker(spec platform.AttachDiskSpec, slot, importPath string) string {
	identity := strings.Join([]string{
		spec.IdempotencyKey,
		slot,
		importPath,
		spec.StorageID,
		string(spec.Format),
		strconv.FormatInt(spec.SizeBytes, 10),
	}, "\x00")
	return diskMarkerPrefix + strings.TrimPrefix(ownershipMarker(identity), ownershipPrefix) + ":" + slot
}
