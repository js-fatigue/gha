package cmd

import (
	"fmt"
	"sort"
)

// CommandFunc is the signature for a registered command handler.
type CommandFunc func() error

var registry = map[string]CommandFunc{}

// Register adds fn under name. Called from init() in each command file.
func Register(name string, fn CommandFunc) {
	registry[name] = fn
}

// Dispatch looks up name in the registry and calls it.
// Returns an error listing available commands if name is not found.
func Dispatch(name string) error {
	fn, ok := registry[name]
	if !ok {
		keys := make([]string, 0, len(registry))
		for k := range registry {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return fmt.Errorf("unknown command %q; registered: %v", name, keys)
	}
	return fn()
}
