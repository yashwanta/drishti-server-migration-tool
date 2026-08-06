package vmware

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/platform"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
)

const maxDescriptorBytes = 4 << 20

type diskDownloader interface {
	Download(ctx context.Context, remotePath, localPath string) error
}

type exportManifest struct {
	VMID       string         `json:"vm_id"`
	DiskID     string         `json:"disk_id"`
	Descriptor string         `json:"descriptor"`
	Files      []manifestFile `json:"files"`
	SHA256     string         `json:"sha256"`
}

type manifestFile struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

// ExportDisk downloads a complete supported VMDK into the worker workspace.
// The VM power state is independently checked in the same API session before
// any datastore data is read.
func (s *Source) ExportDisk(ctx context.Context, vmID, diskID, destPath string) (platform.ExportResult, error) {
	start := time.Now()
	destination, err := validateExportPath(s.workspace, destPath)
	if err != nil {
		return platform.ExportResult{}, err
	}
	client, err := s.client.connect(ctx)
	if err != nil {
		return platform.ExportResult{}, err
	}
	defer func() { _ = client.Logout(context.Background()) }()

	vm, err := retrieveVM(ctx, client.Client, vmID, []string{"config", "runtime.powerState"})
	if err != nil {
		return platform.ExportResult{}, err
	}
	if normalizePower(vm.Runtime.PowerState) != domain.PowerOff {
		return platform.ExportResult{}, fmt.Errorf("VMDK export requires the source VM to be confirmed powered off")
	}
	disk, err := findVirtualDisk(vm, diskID)
	if err != nil {
		return platform.ExportResult{}, err
	}
	backing, ok := disk.Backing.(*types.VirtualDiskFlatVer2BackingInfo)
	if !ok {
		return platform.ExportResult{}, fmt.Errorf("disk %q uses an unsupported VMware backing type %T", diskID, disk.Backing)
	}
	if backing.Parent != nil {
		return platform.ExportResult{}, fmt.Errorf("disk %q has a snapshot/delta backing chain; consolidation is never automatic", diskID)
	}
	if backing.Datastore == nil || strings.TrimSpace(backing.FileName) == "" {
		return platform.ExportResult{}, fmt.Errorf("disk %q backing datastore or filename is unavailable", diskID)
	}
	mode := strings.ToLower(strings.TrimSpace(backing.DiskMode))
	if mode != "" && mode != "persistent" {
		return platform.ExportResult{}, fmt.Errorf("disk %q mode %q is unsupported for cold export", diskID, backing.DiskMode)
	}
	var remote object.DatastorePath
	if !remote.FromString(backing.FileName) || !remote.IsVMDK() {
		return platform.ExportResult{}, fmt.Errorf("disk %q has an invalid datastore VMDK path", diskID)
	}
	ds := object.NewDatastore(client.Client, *backing.Datastore)
	if err := ds.FindInventoryPath(ctx); err != nil {
		return platform.ExportResult{}, fmt.Errorf("resolve backing datastore inventory path: %w", err)
	}
	result, manifest, err := exportVMDK(ctx, objectDatastoreDownloader{datastore: ds}, remote.Path, destination)
	if err != nil {
		return platform.ExportResult{}, err
	}
	manifest.VMID = vmID
	manifest.DiskID = diskID
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return platform.ExportResult{}, fmt.Errorf("encode export manifest: %w", err)
	}
	manifestPath := destination + ".manifest.json"
	if err := writeNewFile(manifestPath, append(manifestBytes, '\n'), 0o600); err != nil {
		removeExportFiles(destination, manifest.Files)
		return platform.ExportResult{}, fmt.Errorf("write export manifest: %w", err)
	}
	result.Duration = time.Since(start).Round(time.Millisecond).String()
	return result, nil
}

func exportVMDK(ctx context.Context, downloader diskDownloader, remoteDescriptor, destination string) (platform.ExportResult, exportManifest, error) {
	tmpSuffix := ".partial-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	tmpDescriptor := destination + tmpSuffix
	if err := downloader.Download(ctx, remoteDescriptor, tmpDescriptor); err != nil {
		return platform.ExportResult{}, exportManifest{}, fmt.Errorf("download VMDK descriptor: %w", err)
	}
	defer os.Remove(tmpDescriptor)
	descriptor, err := readDescriptor(tmpDescriptor)
	if err != nil {
		return platform.ExportResult{}, exportManifest{}, err
	}
	extents, err := parseDescriptorExtents(descriptor)
	if err != nil {
		return platform.ExportResult{}, exportManifest{}, err
	}

	finalFiles := []string{destination}
	tmpFiles := []string{tmpDescriptor}
	remoteDir := path.Dir(remoteDescriptor)
	for _, extent := range extents {
		final := filepath.Join(filepath.Dir(destination), extent)
		if _, err := os.Lstat(final); err == nil {
			return platform.ExportResult{}, exportManifest{}, fmt.Errorf("export extent %q already exists", extent)
		} else if !os.IsNotExist(err) {
			return platform.ExportResult{}, exportManifest{}, fmt.Errorf("inspect export extent %q: %w", extent, err)
		}
		tmp := final + tmpSuffix
		if err := downloader.Download(ctx, path.Join(remoteDir, extent), tmp); err != nil {
			for _, file := range tmpFiles {
				_ = os.Remove(file)
			}
			return platform.ExportResult{}, exportManifest{}, fmt.Errorf("download VMDK extent %q: %w", extent, err)
		}
		tmpFiles = append(tmpFiles, tmp)
		finalFiles = append(finalFiles, final)
	}
	defer func() {
		for _, file := range tmpFiles {
			_ = os.Remove(file)
		}
	}()

	for i := 1; i < len(tmpFiles); i++ {
		if err := os.Rename(tmpFiles[i], finalFiles[i]); err != nil {
			for _, file := range finalFiles[1:i] {
				_ = os.Remove(file)
			}
			return platform.ExportResult{}, exportManifest{}, fmt.Errorf("finalize VMDK extent: %w", err)
		}
	}
	if err := os.Rename(tmpDescriptor, destination); err != nil {
		for _, file := range finalFiles[1:] {
			_ = os.Remove(file)
		}
		return platform.ExportResult{}, exportManifest{}, fmt.Errorf("finalize VMDK descriptor: %w", err)
	}

	manifest, total, combined, err := hashExport(destination, finalFiles)
	if err != nil {
		removeExportFiles(destination, manifest.Files)
		return platform.ExportResult{}, exportManifest{}, err
	}
	return platform.ExportResult{Path: destination, SizeBytes: total, Format: domain.DiskVMDK, SHA256: combined}, manifest, nil
}

func readDescriptor(filename string) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open VMDK descriptor: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxDescriptorBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read VMDK descriptor: %w", err)
	}
	if len(data) > maxDescriptorBytes {
		return nil, fmt.Errorf("VMDK descriptor exceeds %d bytes", maxDescriptorBytes)
	}
	return data, nil
}

func parseDescriptorExtents(descriptor []byte) ([]string, error) {
	var extents []string
	scanner := bufio.NewScanner(strings.NewReader(string(descriptor)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 || (fields[0] != "RW" && fields[0] != "RDONLY" && fields[0] != "NOACCESS") {
			continue
		}
		firstQuote := strings.IndexByte(line, '"')
		lastQuote := strings.LastIndexByte(line, '"')
		if firstQuote < 0 || lastQuote <= firstQuote {
			return nil, fmt.Errorf("VMDK extent line has no quoted filename")
		}
		name := line[firstQuote+1 : lastQuote]
		if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
			return nil, fmt.Errorf("VMDK extent filename %q is unsafe", name)
		}
		extents = append(extents, name)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan VMDK descriptor: %w", err)
	}
	if len(extents) == 0 {
		return nil, fmt.Errorf("VMDK descriptor contains no extents")
	}
	sort.Strings(extents)
	for i := 1; i < len(extents); i++ {
		if extents[i] == extents[i-1] {
			return nil, fmt.Errorf("VMDK descriptor repeats extent %q", extents[i])
		}
	}
	return extents, nil
}

func hashExport(descriptor string, files []string) (exportManifest, int64, string, error) {
	manifest := exportManifest{Descriptor: filepath.Base(descriptor)}
	combined := sha256.New()
	var total int64
	for _, filename := range files {
		file, err := os.Open(filename)
		if err != nil {
			return manifest, 0, "", fmt.Errorf("hash exported VMDK: %w", err)
		}
		individual := sha256.New()
		size, copyErr := io.Copy(io.MultiWriter(individual, combined), file)
		closeErr := file.Close()
		if copyErr != nil {
			return manifest, 0, "", fmt.Errorf("hash exported VMDK: %w", copyErr)
		}
		if closeErr != nil {
			return manifest, 0, "", fmt.Errorf("close exported VMDK: %w", closeErr)
		}
		total += size
		manifest.Files = append(manifest.Files, manifestFile{Name: filepath.Base(filename), SizeBytes: size, SHA256: hex.EncodeToString(individual.Sum(nil))})
	}
	manifest.SHA256 = hex.EncodeToString(combined.Sum(nil))
	return manifest, total, manifest.SHA256, nil
}

func writeNewFile(filename string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(filename)
		return err
	}
	return file.Close()
}

func removeExportFiles(descriptor string, files []manifestFile) {
	_ = os.Remove(descriptor)
	for _, file := range files {
		if file.Name != filepath.Base(descriptor) {
			_ = os.Remove(filepath.Join(filepath.Dir(descriptor), file.Name))
		}
	}
}
