package conversion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/drishti/hypershift-worker/internal/runner"
)

type fakeExecutor struct {
	mu    sync.Mutex
	calls int
	fail  bool
	args  []string
}

func testWorkspace(t *testing.T) string {
	t.Helper()
	workspace, err := os.MkdirTemp(".", ".conversion-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(workspace) })
	return workspace
}

func (f *fakeExecutor) Run(_ context.Context, name string, args []string) (runner.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.args = append([]string(nil), args...)
	if name != "qemu-img" {
		return runner.Result{}, errors.New("unexpected command")
	}
	if f.fail {
		return runner.Result{ExitCode: 1}, errors.New("fixture conversion failure")
	}
	input, err := os.ReadFile(args[5])
	if err != nil {
		return runner.Result{}, err
	}
	if err := os.WriteFile(args[6], append([]byte("converted:"), input...), 0o600); err != nil {
		return runner.Result{}, err
	}
	return runner.Result{ExitCode: 0}, nil
}

func (f *fakeExecutor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestConvertUsesTypedQEMUArgsAndReusesVerifiedOutput(t *testing.T) {
	workspace := testWorkspace(t)
	input := filepath.Join(workspace, "source.vmdk")
	output := filepath.Join(workspace, "job", "target.qcow2")
	if err := os.WriteFile(input, []byte("fixture-vmdk"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := &fakeExecutor{}
	service, err := New(workspace, 1, executor)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{InputPath: input, OutputPath: output, InputFormat: "vmdk", TargetFormat: "qcow2"}
	result, err := service.Convert(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Reused || result.SHA256 == "" || result.SizeBytes == 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	absoluteInput, _ := filepath.Abs(input)
	absoluteOutput, _ := filepath.Abs(output)
	wantArgs := []string{"convert", "-f", "vmdk", "-O", "qcow2", absoluteInput, absoluteOutput + ".drishti-partial"}
	for i := range wantArgs {
		if executor.args[i] != wantArgs[i] {
			t.Fatalf("arg[%d] = %q, want %q", i, executor.args[i], wantArgs[i])
		}
	}
	reused, err := service.Convert(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reused.Reused || reused.SHA256 != result.SHA256 || executor.callCount() != 1 {
		t.Fatalf("idempotent retry did not reuse output: %#v calls=%d", reused, executor.callCount())
	}
}

func TestConcurrentConversionRunsQEMUOnce(t *testing.T) {
	workspace := testWorkspace(t)
	input := filepath.Join(workspace, "source.vmdk")
	output := filepath.Join(workspace, "target.raw")
	if err := os.WriteFile(input, []byte("fixture-vmdk"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := &fakeExecutor{}
	service, err := New(workspace, 1, executor)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{InputPath: input, OutputPath: output, InputFormat: "vmdk", TargetFormat: "raw"}
	var workers sync.WaitGroup
	errorsFound := make(chan error, 16)
	for i := 0; i < 16; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, err := service.Convert(context.Background(), request)
			errorsFound <- err
		}()
	}
	workers.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Errorf("concurrent conversion: %v", err)
		}
	}
	if executor.callCount() != 1 {
		t.Fatalf("qemu-img calls = %d, want 1", executor.callCount())
	}
}

func TestConvertRejectsWorkspaceEscapeAndCleansFailedPartial(t *testing.T) {
	workspace := testWorkspace(t)
	input := filepath.Join(workspace, "source.vmdk")
	if err := os.WriteFile(input, []byte("fixture-vmdk"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := &fakeExecutor{fail: true}
	service, err := New(workspace, 1, executor)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(workspace), "outside.qcow2")
	if _, err := service.Convert(context.Background(), Request{InputPath: input, OutputPath: outside, TargetFormat: "qcow2"}); err == nil {
		t.Fatal("workspace escape accepted")
	}
	output := filepath.Join(workspace, "failed.qcow2")
	if _, err := service.Convert(context.Background(), Request{InputPath: input, OutputPath: output, TargetFormat: "qcow2"}); err == nil {
		t.Fatal("executor failure was not returned")
	}
	if _, err := os.Stat(output + ".drishti-partial"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed partial was retained: %v", err)
	}
}
