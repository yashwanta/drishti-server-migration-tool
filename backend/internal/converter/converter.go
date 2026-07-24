// Package converter handles disk format conversion (vmdk -> qcow2/raw). In
// mock mode it simulates the conversion with a checksum. The real worker uses
// qemu-img via the safelisted runner.
package converter

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Result describes a conversion outcome.
type Result struct {
	InputPath  string
	OutputPath string
	Format     string
	SizeBytes  int64
	SHA256     string
	Duration   string
}

// Convert converts a disk from one format to another. In mock mode it copies
// the input with a format tag and computes a checksum. Real qemu-img conversion
// is performed by the worker runner in lab/production mode.
func Convert(inputPath, outputPath, targetFormat string) (Result, error) {
	start := time.Now()
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return Result{}, fmt.Errorf("read input: %w", err)
	}
	// Append format tag so the output differs from input (simulates conversion).
	tagged := append(data, []byte("|converted:"+targetFormat)...)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return Result{}, fmt.Errorf("create output dir: %w", err)
	}
	if err := os.WriteFile(outputPath, tagged, 0o644); err != nil {
		return Result{}, fmt.Errorf("write output: %w", err)
	}
	h := sha256.Sum256(tagged)
	return Result{
		InputPath:  inputPath,
		OutputPath: outputPath,
		Format:     targetFormat,
		SizeBytes:  int64(len(tagged)),
		SHA256:     hex.EncodeToString(h[:]),
		Duration:   time.Since(start).Round(time.Millisecond).String(),
	}, nil
}

// VerifyChecksum reads a file and returns its SHA256.
func VerifyChecksum(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}