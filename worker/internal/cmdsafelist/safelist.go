// Package cmdsafelist defines the fixed, validated commands the worker is ever
// allowed to execute. The worker never assembles a shell command from
// free-form input; it only invokes these binaries with validated argument
// arrays built from typed, constrained inputs.
package cmdsafelist

import (
	"fmt"
	"strings"
)

// AllowedBinary names a tool the worker may invoke and its fixed path/key.
type AllowedBinary struct {
	Name    string
	Resolve func() string
}

// Known tools. qemu-img has a command-specific conversion grammar below.
var registry = map[string]AllowedBinary{
	"qemu-img": {Name: "qemu-img", Resolve: func() string { return "qemu-img" }},
}

// Resolve returns the executable for name, or an error if it is unknown.
func Resolve(name string) (string, error) {
	b, ok := registry[name]
	if !ok {
		return "", fmt.Errorf("command %q is not in the worker safelist", name)
	}
	return b.Resolve(), nil
}

// Allowed reports whether name is registered.
func Allowed(name string) bool {
	_, ok := registry[name]
	return ok
}

// ValidateArgs enforces a command-specific grammar. Metacharacter rejection in
// the runner is defense in depth; this function prevents adding arbitrary
// qemu-img subcommands or flags through a safelisted binary.
func ValidateArgs(name string, args []string) error {
	switch name {
	case "qemu-img":
		if len(args) != 7 || args[0] != "convert" || args[1] != "-f" || args[2] != "vmdk" || args[3] != "-O" {
			return fmt.Errorf("qemu-img arguments do not match the conversion safelist")
		}
		if args[4] != "raw" && args[4] != "qcow2" {
			return fmt.Errorf("qemu-img output format %q is not allowed", args[4])
		}
		for _, path := range args[5:] {
			if strings.HasPrefix(path, "-") {
				return fmt.Errorf("qemu-img path cannot be parsed as an option")
			}
		}
		return nil
	default:
		return fmt.Errorf("command %q is not in the worker safelist", name)
	}
}
