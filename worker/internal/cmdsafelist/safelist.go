// Package cmdsafelist defines the fixed, validated commands the worker is ever
// allowed to execute. The worker never assembles a shell command from
// free-form input; it only invokes these binaries with validated argument
// arrays built from typed, constrained inputs.
package cmdsafelist

import "fmt"

// AllowedBinary names a tool the worker may invoke and its fixed path/key.
type AllowedBinary struct {
	Name    string
	Resolve func() string
}

// Known tools. Phase 0 declares qemu-img and sha256sum as the conversion and
// integrity tools. virt-v2v is added in Phase 5 once validated.
var registry = map[string]AllowedBinary{
	"qemu-img":   {Name: "qemu-img", Resolve: func() string { return "qemu-img" }},
	"sha256sum":  {Name: "sha256sum", Resolve: func() string { return "sha256sum" }},
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