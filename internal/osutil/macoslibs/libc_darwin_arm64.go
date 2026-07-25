//go:build darwin && arm64

package macoslibs

type syslog1 = func(
	priority int, message string,
	_, _, _, _, _, _ uintptr, // this is a hack to force variardic args to be on a stack
	arg0 uintptr,
)

func (l *Libc) Syslog1(priority int, message string, arg0 uintptr) {
	l.syslog1(priority, message, 0, 0, 0, 0, 0, 0, arg0)
}
