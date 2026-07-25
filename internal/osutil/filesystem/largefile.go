//go:build linux || solaris || aix || zos

package filesystem

import "golang.org/x/sys/unix"

// openLargeFile is O_LARGEFILE on 32-bit targets (0x8000 on 386, 0x20000 on
// arm, ...) and 0 on 64-bit ones. Without it, os.Root's openat rejects files
// larger than 2 GiB on a 32-bit kernel with EOVERFLOW ("value too large for
// defined data type"), which breaks streaming of PS3 ISOs on legacy 32-bit NAS
// hardware. The constant is only defined by x/sys/unix on the platforms in the
// build constraint above (Linux, Solaris, AIX, z/OS); see largefile_other.go
// for the rest.
const openLargeFile = unix.O_LARGEFILE
