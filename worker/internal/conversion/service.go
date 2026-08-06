// Package conversion owns validated qemu-img disk conversion on the worker.
package conversion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/drishti/hypershift-worker/internal/runner"
)

const gibibyte = int64(1024 * 1024 * 1024)

type Executor interface {
	Run(context.Context, string, []string) (runner.Result, error)
}

type Request struct {
	InputPath    string `json:"input_path"`
	OutputPath   string `json:"output_path"`
	InputFormat  string `json:"input_format"`
	TargetFormat string `json:"target_format"`
}

type Result struct {
	InputPath  string `json:"input_path"`
	OutputPath string `json:"output_path"`
	Format     string `json:"format"`
	SizeBytes  int64  `json:"size_bytes"`
	SHA256     string `json:"sha256"`
	Duration   string `json:"duration"`
	Reused     bool   `json:"reused"`
}

type manifest struct {
	InputSHA256  string `json:"input_sha256"`
	OutputSHA256 string `json:"output_sha256"`
	TargetFormat string `json:"target_format"`
	SizeBytes    int64  `json:"size_bytes"`
}

// Service serializes conversions conservatively. qemu-img remains a separate
// process, but only typed requests beneath workspaceRoot can start it.
type Service struct {
	workspaceRoot string
	maxInputBytes int64
	executor      Executor
	mu            sync.Mutex
}

func New(workspaceRoot string, maxWorkspaceGB int64, executor Executor) (*Service, error) {
	if executor == nil {
		return nil, fmt.Errorf("conversion executor is required")
	}
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("create workspace root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace symlinks: %w", err)
	}
	if maxWorkspaceGB <= 0 {
		return nil, fmt.Errorf("workspace size cap must be positive")
	}
	return &Service{workspaceRoot: root, maxInputBytes: maxWorkspaceGB * gibibyte, executor: executor}, nil
}

func (s *Service) Convert(ctx context.Context, request Request) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	started := time.Now()

	if request.InputFormat == "" {
		request.InputFormat = "vmdk"
	}
	if request.InputFormat != "vmdk" {
		return Result{}, fmt.Errorf("input format must be vmdk")
	}
	if request.TargetFormat != "raw" && request.TargetFormat != "qcow2" {
		return Result{}, fmt.Errorf("target format must be raw or qcow2")
	}
	inputPath, err := s.existingFile(request.InputPath)
	if err != nil {
		return Result{}, fmt.Errorf("input path: %w", err)
	}
	inputInfo, err := os.Stat(inputPath)
	if err != nil {
		return Result{}, err
	}
	if inputInfo.Size() > s.maxInputBytes {
		return Result{}, fmt.Errorf("input exceeds configured workspace size cap")
	}
	outputPath, err := s.outputFile(request.OutputPath)
	if err != nil {
		return Result{}, fmt.Errorf("output path: %w", err)
	}
	if samePath(inputPath, outputPath) {
		return Result{}, fmt.Errorf("input and output paths must differ")
	}
	inputChecksum, err := checksum(inputPath)
	if err != nil {
		return Result{}, fmt.Errorf("checksum input: %w", err)
	}
	if reused, ok := s.reusable(outputPath, request.TargetFormat, inputChecksum); ok {
		reused.InputPath = inputPath
		reused.Duration = time.Since(started).Round(time.Millisecond).String()
		reused.Reused = true
		return reused, nil
	}
	if _, err := os.Lstat(outputPath); err == nil {
		return Result{}, fmt.Errorf("output already exists without a matching DRISHTI manifest")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, err
	}

	partialPath := outputPath + ".drishti-partial"
	if err := removeRegularFile(partialPath); err != nil {
		return Result{}, err
	}
	commandResult, runErr := s.executor.Run(ctx, "qemu-img", []string{
		"convert", "-f", "vmdk", "-O", request.TargetFormat, inputPath, partialPath,
	})
	if runErr != nil || commandResult.ExitCode != 0 {
		_ = removeRegularFile(partialPath)
		if runErr != nil {
			return Result{}, fmt.Errorf("qemu-img conversion failed: %w", runErr)
		}
		return Result{}, fmt.Errorf("qemu-img conversion failed with exit code %d", commandResult.ExitCode)
	}
	inputChecksumAfter, err := checksum(inputPath)
	if err != nil || inputChecksumAfter != inputChecksum {
		_ = removeRegularFile(partialPath)
		return Result{}, fmt.Errorf("input changed while conversion was running")
	}
	if err := os.Rename(partialPath, outputPath); err != nil {
		_ = removeRegularFile(partialPath)
		return Result{}, fmt.Errorf("publish converted disk: %w", err)
	}
	outputInfo, err := os.Stat(outputPath)
	if err != nil || !outputInfo.Mode().IsRegular() {
		return Result{}, fmt.Errorf("converted output is not a regular file")
	}
	if outputInfo.Size() > s.maxInputBytes {
		_ = os.Remove(outputPath)
		return Result{}, fmt.Errorf("converted output exceeds configured workspace size cap")
	}
	outputChecksum, err := checksum(outputPath)
	if err != nil {
		return Result{}, fmt.Errorf("checksum output: %w", err)
	}
	metadata := manifest{InputSHA256: inputChecksum, OutputSHA256: outputChecksum, TargetFormat: request.TargetFormat, SizeBytes: outputInfo.Size()}
	if err := writeManifest(outputPath+".drishti.json", metadata); err != nil {
		return Result{}, fmt.Errorf("write conversion manifest: %w", err)
	}
	return Result{InputPath: inputPath, OutputPath: outputPath, Format: request.TargetFormat,
		SizeBytes: outputInfo.Size(), SHA256: outputChecksum, Duration: time.Since(started).Round(time.Millisecond).String()}, nil
}

func (s *Service) existingFile(path string) (string, error) {
	resolved, err := s.resolveWithin(path, true)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("must be a regular non-symlink file")
	}
	return resolved, nil
}

func (s *Service) outputFile(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err := s.requireWithin(abs); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", err
	}
	resolved := filepath.Join(parent, filepath.Base(abs))
	if err := s.requireWithin(resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

func (s *Service) resolveWithin(path string, evaluate bool) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if evaluate {
		abs, err = filepath.EvalSymlinks(abs)
		if err != nil {
			return "", err
		}
	}
	if err := s.requireWithin(abs); err != nil {
		return "", err
	}
	return abs, nil
}

func (s *Service) requireWithin(path string) error {
	relative, err := filepath.Rel(s.workspaceRoot, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("path escapes the worker workspace")
	}
	return nil
}

func (s *Service) reusable(outputPath, format, inputChecksum string) (Result, bool) {
	info, err := os.Lstat(outputPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return Result{}, false
	}
	data, err := os.ReadFile(outputPath + ".drishti.json")
	if err != nil {
		return Result{}, false
	}
	var metadata manifest
	if json.Unmarshal(data, &metadata) != nil || metadata.InputSHA256 != inputChecksum || metadata.TargetFormat != format || metadata.SizeBytes != info.Size() {
		return Result{}, false
	}
	actualChecksum, err := checksum(outputPath)
	if err != nil || actualChecksum != metadata.OutputSHA256 {
		return Result{}, false
	}
	return Result{OutputPath: outputPath, Format: format, SizeBytes: info.Size(), SHA256: actualChecksum}, true
}

func writeManifest(path string, metadata manifest) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	temporary := path + ".partial"
	if err := removeRegularFile(temporary); err != nil {
		return err
	}
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(temporary)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func removeRegularFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to remove non-regular partial output")
	}
	return os.Remove(path)
}

func checksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func samePath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}
