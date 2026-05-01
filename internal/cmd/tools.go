//go:build tools

// Package cmd anchors module dependencies that are referenced by upcoming
// tasks but not yet imported by production code. The `tools` build tag keeps
// these imports out of normal builds while preventing `go mod tidy` from
// pruning them. Remove entries from here as real call sites land.
package cmd

import (
	_ "github.com/alecthomas/kong"
)
