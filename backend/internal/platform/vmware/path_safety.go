package vmware

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validateExportPath(workspace, destination string) (string, error) {
	if workspace == "" || destination == "" {
		return "", fmt.Errorf("workspace and destination are required")
	}
	root, err := filepath.Abs(workspace)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	dest, err := filepath.Abs(destination)
	if err != nil {
		return "", fmt.Errorf("resolve export destination: %w", err)
	}
	rel, err := filepath.Rel(root, dest)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("export destination must be beneath the worker workspace")
	}
	if !strings.EqualFold(filepath.Ext(dest), ".vmdk") {
		return "", fmt.Errorf("export destination must have a .vmdk extension")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return "", fmt.Errorf("create export directory: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace links: %w", err)
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(dest))
	if err != nil {
		return "", fmt.Errorf("resolve export directory links: %w", err)
	}
	rel, err = filepath.Rel(resolvedRoot, resolvedParent)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("export directory escapes the worker workspace")
	}
	if _, err := os.Lstat(dest); err == nil {
		return "", fmt.Errorf("export destination already exists")
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect export destination: %w", err)
	}
	return dest, nil
}
