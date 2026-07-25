package macoslibs

import (
	"sync"
	"unsafe"

	"github.com/xakep666/ps3netsrv-go/internal/osutil/dynamiclibs"
)

const (
	// Options

	// From /usr/include/sys/syslog.h.
	// These are the same on Linux, BSD, and OS X.

	LOG_PID    = 1 << iota // log the pid with each message
	LOG_CONS               // log on the console if errors in sending
	LOG_ODELAY             // delay open until first syslog() (default)
	LOG_NDELAY             // don't delay open
	LOG_NOWAIT             // don't wait for console forks: DEPRECATED
	LOG_PERROR             // log to stderr as well
)

const (
	// Severity.

	// From /usr/include/sys/syslog.h.
	// These are the same on Linux, BSD, and OS X.
	LOG_EMERG = iota
	LOG_ALERT
	LOG_CRIT
	LOG_ERR
	LOG_WARNING
	LOG_NOTICE
	LOG_INFO
	LOG_DEBUG
)

const (
	// Facility.

	// From /usr/include/sys/syslog.h.
	// These are the same up to LOG_FTP on Linux, BSD, and OS X.
	LOG_KERN = iota << 3
	LOG_USER
	LOG_MAIL
	LOG_DAEMON
	LOG_AUTH
	LOG_SYSLOG
	LOG_LPR
	LOG_NEWS
	LOG_UUCP
	LOG_CRON
	LOG_AUTHPRIV
	LOG_FTP
	_ // unused
	_ // unused
	_ // unused
	_ // unused
	LOG_LOCAL0
	LOG_LOCAL1
	LOG_LOCAL2
	LOG_LOCAL3
	LOG_LOCAL4
	LOG_LOCAL5
	LOG_LOCAL6
	LOG_LOCAL7
)

type Libc struct {
	free     func(unsafe.Pointer)
	openlog  func(ident string, logopt, facility int)
	closelog func()
	syslog1  syslog1
}

var getLibc = sync.OnceValues(func() (*Libc, error) {
	handle, err := dynamiclibs.LoadSystemLibrary("/usr/lib/libSystem.B.dylib")
	if err != nil {
		return nil, err
	}

	ret := new(Libc)
	dynamiclibs.RegisterLibFunc(&ret.free, handle, "free")
	dynamiclibs.RegisterLibFunc(&ret.openlog, handle, "openlog")
	dynamiclibs.RegisterLibFunc(&ret.closelog, handle, "closelog")
	dynamiclibs.RegisterLibFunc(&ret.syslog1, handle, "syslog")

	return ret, nil
})

func GetLibc() (*Libc, error) {
	return getLibc()
}

func (l *Libc) Free(ptr unsafe.Pointer) {
	l.free(ptr)
}

func (l *Libc) OpenLog(ident string, logopt, facility int) {
	l.openlog(ident, logopt, facility)
}

func (l *Libc) CloseLog() {
	l.closelog()
}
