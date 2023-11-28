package logutil

import (
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
	return slog.StringValue(addrToLog(la.addr))
}

func addrToLog(addr net.Addr) string {
	tcpAddr, isTcpAddr := addr.(*net.TCPAddr)
	if !isTcpAddr {
		return addr.String()
	}

	// if bound to all, print first non-localhost ip
	// ipv6 with zone handled separately
	isV4Any := tcpAddr.IP.Equal(net.IPv4zero)
	isV6Any := tcpAddr.IP.Equal(net.IPv6unspecified)
	if isV4Any || (isV6Any && tcpAddr.Zone == "") {
		ifaddrs, err := net.InterfaceAddrs()
		if err != nil {
			return addr.String()
		}

		if foundAddr := firstSuitableIfaddr(ifaddrs, isV4Any); foundAddr != "" {
			return net.JoinHostPort(foundAddr, strconv.Itoa(tcpAddr.Port))
		}
	}

	// for zoned addr try to get interface by name
	if isV6Any && tcpAddr.Zone != "" {
		iface, err := net.InterfaceByName(tcpAddr.Zone)
		if err != nil {
			return addr.String()
		}

		ifaddrs, err := iface.Addrs()
		if err != nil {
			return addr.String()
		}

		if foundAddr := firstSuitableIfaddr(ifaddrs, isV4Any); foundAddr != "" {
			return net.JoinHostPort(foundAddr, strconv.Itoa(tcpAddr.Port))
		}
	}

	return addr.String()
}

func firstSuitableIfaddr(ifaddrs []net.Addr, skipV6 bool) string {
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
		if skipV6 && len(ipNet.IP) == net.IPv6len {
			continue
		}

		return ipNet.IP.String()
	}

	return ""
}
