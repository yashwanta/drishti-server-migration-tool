package converter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWorkerClientUsesTypedConversionEndpointConcurrently(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/conversions" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		mu.Lock()
		requests++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_path":"/workspace/in.vmdk","output_path":"/workspace/out.qcow2","format":"qcow2","size_bytes":10,"sha256":"fixture","duration":"1ms"}`))
	}))
	defer server.Close()
	client, err := NewWorkerClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for i := 0; i < 32; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if _, err := client.Convert(context.Background(), "/workspace/in.vmdk", "/workspace/out.qcow2", "qcow2"); err != nil {
				t.Errorf("Convert: %v", err)
			}
		}()
	}
	workers.Wait()
	mu.Lock()
	defer mu.Unlock()
	if requests != 32 {
		t.Fatalf("requests = %d", requests)
	}
}

func TestMockConversionAndStreamingChecksum(t *testing.T) {
	input := filepath.Join(t.TempDir(), "input.vmdk")
	output := filepath.Join(t.TempDir(), "output.raw")
	if err := os.WriteFile(input, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := NewMock().Convert(context.Background(), input, output, "raw")
	if err != nil {
		t.Fatal(err)
	}
	checksum, err := VerifyChecksum(output)
	if err != nil || checksum != result.SHA256 {
		t.Fatalf("checksum=%q result=%q err=%v", checksum, result.SHA256, err)
	}
}

func TestWorkerClientRejectsEmbeddedCredentials(t *testing.T) {
	if _, err := NewWorkerClient("http://user:password@worker:8090", nil); err == nil {
		t.Fatal("worker URL with embedded credentials accepted")
	}
}
