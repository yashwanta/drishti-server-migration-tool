// Package runner executes allowlisted commands with validated argument arrays.
// It never invokes a shell and never interpolates free-form input into a
// command string.
package runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/drishti/hypershift-worker/internal/cmdsafelist"
)

// Result is the sanitized outcome of a command run.
type Result struct {
	ExitCode int
	Duration time.Duration
	Stdout   string
	Stderr   string
}

// Runner executes allowlisted commands.
type Runner struct {
	timeout time.Duration
}

func New(timeout time.Duration) *Runner { return &Runner{timeout: timeout} }

// Run executes the named allowlisted binary with the given validated args.
func (r *Runner) Run(ctx context.Context, name string, args []string) (Result, error) {
	exe, err := cmdsafelist.Resolve(name)
	if err != nil {
		return Result{}, err
	}
	for _, a := range args {
		if err := validateArg(a); err != nil {
			return Result{}, fmt.Errorf("invalid argument to %s: %w", name, err)
		}
	}
	if err := cmdsafelist.ValidateArgs(name, args); err != nil {
		return Result{}, fmt.Errorf("invalid argument set for %s: %w", name, err)
	}

	cctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, exe, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err = cmd.Run()
	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	res := Result{
		ExitCode: exitCode,
		Duration: time.Since(start),
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
	}
	if err != nil {
		return res, fmt.Errorf("%s failed with exit code %d: %w", name, res.ExitCode, err)
	}
	return res, nil
}

// validateArg rejects shell metacharacters and command separators.
func validateArg(a string) error {
	if a == "" {
		return fmt.Errorf("empty argument")
	}
	for _, c := range ";|&`$\n\r" {
		if strings.ContainsRune(a, c) {
			return fmt.Errorf("argument contains forbidden character %q", string(c))
		}
	}
	return nil
}
