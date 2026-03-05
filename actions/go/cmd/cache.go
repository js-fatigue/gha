package cmd

import ac "github.com/bshore/gha/internal/actions-core"

func init() { Register("cache", ac.RunCache) }
