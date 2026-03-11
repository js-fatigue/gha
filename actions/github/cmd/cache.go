package cmd

import ac "github.com/js-fatigue/gha/internal/actions-core"

// RunCache is a function used internally by all action families.
// The actions/github family is the only one that exports it as an
// action command, usable by callers.
func init() { Register("cache", ac.RunCache) }
