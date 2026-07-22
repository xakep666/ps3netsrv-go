package socketactivation

import (
	"fmt"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

var onceActviationListeners = sync.OnceValues(
	func() (map[string][]net.Listener, error) {
		fds, err := activationFiles()
		if err != nil {
			return nil, err
		}

		listeners := make(map[string][]net.Listener, len(fds))
		for _, fd := range fds {
			if lis, err := net.FileListener(fd); err == nil {
				name := fd.Name()
				listeners[name] = append(listeners[name], lis)
			}
		}

		return listeners, nil
	},
)

func ActivationListeners(name string) ([]net.Listener, error) {
	listeners, err := onceActviationListeners()
	if err != nil {
		return nil, err
	}

	return slices.Clone(listeners[name]), nil
}

func activationFiles() ([]*os.File, error) {
	defer func() {
		_ = os.Unsetenv("LISTEN_PID")
		_ = os.Unsetenv("LISTEN_FDS")
		_ = os.Unsetenv("LISTEN_FDNAMES")
	}()

	pidValue, ok := os.LookupEnv("LISTEN_PID")
	if !ok {
		return nil, nil
	}

	pid, err := strconv.Atoi(pidValue)
	if err != nil {
		return nil, fmt.Errorf("get listen pid: %w", err)
	}

	// prevent running inside child forked processes
	if os.Getpid() != pid {
		return nil, nil
	}

	nfdsValue, ok := os.LookupEnv("LISTEN_FDS")
	if !ok {
		return nil, nil
	}

	nfds, err := strconv.Atoi(nfdsValue)
	if err != nil {
		return nil, fmt.Errorf("get listen fds count: %w", err)
	}

	fdnames := os.Getenv("LISTEN_FDNAMES")

	const firstListenFd = 3 // systemd passes listen fds starting from 3 (0,1,2 are reserved for stdin,stdout,stderr)
	files := make([]*os.File, nfds)
	for i := range nfds {
		var name string
		name, fdnames, _ = strings.Cut(fdnames, ":")

		fd := i + firstListenFd
		if name == "" {
			name = "LISTEN_FD_" + strconv.Itoa(fd)
		}

		syscall.CloseOnExec(fd) // avoid inheritance by child forks
		files[i] = os.NewFile(uintptr(fd), name)
	}

	return files, nil
}
