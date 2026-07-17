//go:build !linux

package fs

// openLargeFile has no effect on non-Linux platforms; large-file access there
// does not depend on an explicit open flag.
const openLargeFile = 0
