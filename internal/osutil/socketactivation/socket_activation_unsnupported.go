//go:build !linux && !darwin

package socketactivation

import (
	"errors"
	"net"
)

func ActivationListeners(name string) ([]net.Listener, error) {
	return nil, errors.ErrUnsupported
}
