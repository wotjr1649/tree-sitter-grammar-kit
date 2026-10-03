//go:build !windows

package main

import "os"

// isAlias reports a symlink left in a resolved path (EvalSymlinks normally removes them).
func isAlias(_ string, info os.FileInfo) bool { return info.Mode()&os.ModeSymlink != 0 }
