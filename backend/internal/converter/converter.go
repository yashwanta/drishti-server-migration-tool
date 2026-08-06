// Package converter defines the orchestrator-to-worker disk conversion boundary.
package converter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Result struct {
	InputPath  string `json:"input_path"`
	OutputPath string `json:"output_path"`
	Format     string `json:"format"`
	SizeBytes  int64  `json:"size_bytes"`
	SHA256     string `json:"sha256"`
	Duration   string `json:"duration"`
	Reused     bool   `json:"reused"`
}

type DiskConverter interface {
	Convert(context.Context, string, string, string) (Result, error)
}

// Mock performs the checksum-based simulation used only in mock mode.
type Mock struct{}

func NewMock() *Mock { return &Mock{} }

func (m *Mock) Convert(_ context.Context, inputPath, outputPath, targetFormat string) (Result, error) {
	start := time.Now()
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return Result{}, fmt.Errorf("read input: %w", err)
	}
	tagged := append(data, []byte("|converted:"+targetFormat)...)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return Result{}, fmt.Errorf("create output dir: %w", err)
	}
	if err := os.WriteFile(outputPath, tagged, 0o644); err != nil {
		return Result{}, fmt.Errorf("write output: %w", err)
	}
	hash := sha256.Sum256(tagged)
	return Result{InputPath: inputPath, OutputPath: outputPath, Format: targetFormat,
		SizeBytes: int64(len(tagged)), SHA256: hex.EncodeToString(hash[:]),
		Duration: time.Since(start).Round(time.Millisecond).String()}, nil
}

// Convert is retained for mock-only callers. Real modes inject WorkerClient.
func Convert(inputPath, outputPath, targetFormat string) (Result, error) {
	return NewMock().Convert(context.Background(), inputPath, outputPath, targetFormat)
}

type WorkerClient struct {
	endpoint string
	client   *http.Client
}

func NewWorkerClient(endpoint string, client *http.Client) (*WorkerClient, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return nil, fmt.Errorf("worker URL must be an http(s) endpoint without embedded credentials")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("worker URL cannot contain a query or fragment")
	}
	if client == nil {
		client = &http.Client{}
	}
	return &WorkerClient{endpoint: strings.TrimRight(parsed.String(), "/"), client: client}, nil
}

func (c *WorkerClient) Convert(ctx context.Context, inputPath, outputPath, targetFormat string) (Result, error) {
	payload, err := json.Marshal(map[string]string{
		"input_path": inputPath, "output_path": outputPath, "input_format": "vmdk", "target_format": targetFormat,
	})
	if err != nil {
		return Result{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/api/v1/conversions", bytes.NewReader(payload))
	if err != nil {
		return Result{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return Result{}, fmt.Errorf("worker conversion request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &failure)
		if failure.Error == "" {
			failure.Error = http.StatusText(response.StatusCode)
		}
		return Result{}, fmt.Errorf("worker refused conversion: %s", failure.Error)
	}
	var result Result
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&result); err != nil {
		return Result{}, fmt.Errorf("decode worker conversion result: %w", err)
	}
	if result.OutputPath == "" || result.SHA256 == "" || result.Format != targetFormat {
		return Result{}, fmt.Errorf("worker returned incomplete conversion evidence")
	}
	return result, nil
}

func VerifyChecksum(path string) (string, error) {
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

var _ DiskConverter = (*Mock)(nil)
var _ DiskConverter = (*WorkerClient)(nil)
