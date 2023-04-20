package iprange

import "net"

// FilterListener returns listener that immediately drops accepted connections if peer address is not in provided ip range.
// Or it drops connection for peers with address in provided range if 'invert' specified.
func FilterListener(l net.Listener, r *IPRange, invert bool) net.Listener {
	return &filteringListener{
		Listener: l,
		r:        r,
		invert:   invert,
	}
}

type filteringListener struct {
	net.Listener

	r      *IPRange
	invert bool
}

func (l *filteringListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return conn, err
		}

		shouldAccept := true
		switch addr := conn.RemoteAddr().(type) {
		case *net.TCPAddr:
			shouldAccept = l.shouldAccept(addr.IP)
		case *net.IPAddr:
			shouldAccept = l.shouldAccept(addr.IP)
		case *net.UDPAddr:
			shouldAccept = l.shouldAccept(addr.IP)
		}

		if !shouldAccept {
			_ = conn.Close()
			continue
		}

		return conn, nil
	}
}

func (l *filteringListener) shouldAccept(ip net.IP) bool {
	ok := l.r.Contains(ip)
	if l.invert {
		ok = !ok
	}

	return ok
}
