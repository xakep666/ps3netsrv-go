//go:build darwin && !arm64

package macoslibs

type syslog1 = func(priority int, message string, arg0 uintptr)

func (l *Libc) Syslog1(priority int, message string, arg0 uintptr) {
	l.syslog1(priority, message, arg0)
}
