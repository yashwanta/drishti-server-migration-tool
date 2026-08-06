package runner

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunRejectsUnknownCommand(t *testing.T) {
	r := New(5 * time.Second)
	_, err := r.Run(context.Background(), "rm", []string{"-rf", "/"})
	if err == nil {
		t.Fatal("expected error for non-allowlisted command")
	}
	if !strings.Contains(err.Error(), "safelist") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunRejectsMetachars(t *testing.T) {
	r := New(5 * time.Second)
	_, err := r.Run(context.Background(), "qemu-img", []string{"convert", "-f", "vmdk", "-O", "qcow2", "info; rm -rf /", "/tmp/out"})
	if err == nil {
		t.Fatal("expected error for argument with metachar")
	}
}

func TestValidateArg(t *testing.T) {
	cases := []struct {
		arg   string
		valid bool
	}{
		{"/tmp/file.qcow2", true},
		{"info", true},
		{"", false},
		{"a; b", false},
		{"a | b", false},
		{"a & b", false},
		{"a`b", false},
		{"a$VAR", false},
	}
	for _, c := range cases {
		err := validateArg(c.arg)
		if c.valid && err != nil {
			t.Errorf("expected valid for %q, got %v", c.arg, err)
		}
		if !c.valid && err == nil {
			t.Errorf("expected invalid for %q", c.arg)
		}
	}
}
