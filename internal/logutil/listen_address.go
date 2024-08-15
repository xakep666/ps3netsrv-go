package logutil

import (
	"iter"
	"log/slog"
	"net"
	"strconv"
)

// ListenAddressValue prints addres suitable to for connection.
// For example for "0.0.0.0:8080" it will be output "192.168.2.3:8080" if first non-localhost interface address is "192.168.2.3".
func ListenAddressValue(addr net.Addr) slog.Value {
	return slog.AnyValue(&listenAddress{addr})
}

type listenAddress struct {
	addr net.Addr
}

func (la *listenAddress) LogValue() slog.Value {
	return slog.AnyValue(addrToLog(la.addr))
}

func addrToLog(addr net.Addr) []string {
	tcpAddr, isTcpAddr := addr.(*net.TCPAddr)
	if !isTcpAddr {
		return []string{addr.String()}
	}

	// if bound to all, print first non-localhost ip
	// ipv6 with zone handled separately
	isV4Any := tcpAddr.IP.Equal(net.IPv4zero)
	isV6Any := tcpAddr.IP.Equal(net.IPv6unspecified)
	if isV4Any || (isV6Any && tcpAddr.Zone == "") {
		ifaddrs, err := net.InterfaceAddrs()
		if err != nil {
			return []string{addr.String()}
		}

		ret := make([]string, 0, len(ifaddrs))
		for addr := range allSuitableAddrs(ifaddrs, isV4Any) {
			ret = append(ret, net.JoinHostPort(addr, strconv.Itoa(tcpAddr.Port)))
		}

		return ret
	}

	// for zoned addr try to get interface by name
	if isV6Any && tcpAddr.Zone != "" {
		iface, err := net.InterfaceByName(tcpAddr.Zone)
		if err != nil {
			return []string{addr.String()}
		}

		ifaddrs, err := iface.Addrs()
		if err != nil {
			return []string{addr.String()}
		}

		ret := make([]string, 0, len(ifaddrs))
		for addr := range allSuitableAddrs(ifaddrs, false) {
			ret = append(ret, net.JoinHostPort(addr, strconv.Itoa(tcpAddr.Port)))
		}

		return ret
	}

	return []string{addr.String()}
}

func allSuitableAddrs(ifaddrs []net.Addr, skipV6 bool) iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, ifaddr := range ifaddrs {
			ipNet, isIPNet := ifaddr.(*net.IPNet)
			if !isIPNet {
				continue
			}

			// skip loopback
			if ipNet.IP.IsLoopback() {
				continue
			}

			// skip v6 for v4 bound
			if skipV6 && ipNet.IP.To4() == nil {
				continue
			}

			if !yield(ipNet.IP.String()) {
				return
			}
		}
	}
}
