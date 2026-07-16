//go:build !linux

package osutil

import (
	"errors"
	"net"
)

func ActivationListeners(name string) ([]net.Listener, error) {
	return nil, errors.ErrUnsupported
}
