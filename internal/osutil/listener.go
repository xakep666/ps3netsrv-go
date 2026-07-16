package osutil

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

// MakeListener can create listener from multiple sources:
// * if "addr" starts with "fd:" - from inherited file descriptor
// * if "addr" starts with "activated:" - searches inherited file descriptor by name in socket-activated environment
// * if "addr" starts with "unix:" - unix domain sockets (files or abstract), useful if server is behind a proxy
// * regular tcp listener otherwise
func MakeListener(addr string) (net.Listener, error) {
	const (
		fdPrefix        = "fd:"
		activatedPrefix = "activated:"
		unixPrefix      = "unix:"
	)

	switch {
	case strings.HasPrefix(addr, fdPrefix): // listen on inherited file descriptor
		fd, err := strconv.ParseUint(addr[len(fdPrefix):], 10, strconv.IntSize)
		if err != nil {
			return nil, fmt.Errorf("parse fd: %w", err)
		}

		f := os.NewFile(uintptr(fd), "")
		defer f.Close() // can be safely closed here because FileListener dups fd with necessary opts

		return net.FileListener(f)
	case strings.HasPrefix(addr, activatedPrefix):
		// search a "named" file descriptor if we're inside socket-activated environment
		listeners, err := ActivationListeners(addr[len(activatedPrefix):])
		if err != nil {
			return nil, fmt.Errorf("get activated listeners: %w", err)
		}

		if len(listeners) == 0 {
			return nil, fmt.Errorf("no activated listeners found by name")
		}

		return listeners[0], nil
	case strings.HasPrefix(addr, unixPrefix):
		path := addr[len(unixPrefix):]
		if strings.HasPrefix(path, "@") {
			// abstract unix socket
			return net.Listen("unix", path)
		}

		path, perm, _ := strings.Cut(path, ",") // get permissions
		_ = os.Remove(path)                     // remove existing file
		lis, err := net.Listen("unix", path)
		if err != nil {
			return nil, err
		}

		if parsedPerm, err := strconv.ParseUint(perm, 8, 32); err == nil {
			// apply permissions to file
			if err = os.Chmod(path, os.FileMode(parsedPerm)); err != nil {
				return nil, fmt.Errorf("chmod: %w", err)
			}
		}

		return lis, nil
	}

	// if address is ipv4 we should pass "tcp4" net to listen only on ipv4 addresses

	host, _, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		return net.Listen("tcp", addr)
	}

	ipAddr, err := netip.ParseAddr(host)
	if err != nil {
		return net.Listen("tcp", addr)
	}

	if ipAddr.Is4() {
		return net.Listen("tcp4", addr)
	}

	return net.Listen("tcp", addr)
}
