//go:build !(linux || solaris || aix || zos)

package osutil

// openLargeFile has no effect on platforms where x/sys/unix does not define
// O_LARGEFILE (darwin, the BSDs, Windows, ...); large-file access there does
// not depend on an explicit open flag.
const openLargeFile = 0
