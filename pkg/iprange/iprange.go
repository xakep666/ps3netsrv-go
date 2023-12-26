package iprange

import (
	"bytes"
	"fmt"
	"net"
	"strconv"
)

// IPRange represents range of IP addresses.
type IPRange struct {
	left, right net.IP
}

// ParseIPRange parses strings in following format to IP range:
// * IPv4 single address ("192.168.0.2")
// * IPv4 CIDR ("192.168.0.1/24")
// * IPv4 with subnet mask ("192.168.0.1/255.255.255.0")
// * IPv4 range ("192.168.0.1-192.168.0.255"), both bounds are included, left must be less or equal to right one
// * IPv6 single address ("2001:db8::1")
// * IPv6 CIDR ("2001:db8::/64")
// * IPv6 range ("2001:db8::-2001:db8::10"), both bounds are included, left must be less or equal to right one
// IPv4 CIDR, IPv4 with subnet mask and IPv6 CIDR ranges don't include network and broadcast addresses.
// IPv4 addresses handled as IPv6-mapped addresses (i.e. "::ffff:192.0.2.1") for simplicity.
func ParseIPRange(s string) (*IPRange, error) {
	var ret *IPRange
	// find separator and decide how to parse
sepLoop:
	for i, c := range s {
		switch c {
		case '/': // cidr or netmask
			ret = parseCIDRorMask(s, i)
			break sepLoop
		case '-': // two addresses separated
			ret = parseTwo(s, i)
			break sepLoop
		}
	}

	// maybe it's a single address
	if ret == nil {
		if singleAddr := net.ParseIP(s); singleAddr != nil {
			ret = &IPRange{left: singleAddr, right: singleAddr}
		}
	}

	if ret == nil {
		return nil, fmt.Errorf("invalid syntax of range %q", s)
	}

	return ret, nil
}

// New constructs IPRange from bounds.
// This function returns nil in following cases:
// * one of addresses is nil
// * left bound greater than right one
// * addresses has different sizes
func New(left, right net.IP) *IPRange {
	if len(left) != len(right) {
		return nil
	}

	left, right = left.To16(), right.To16() // canonicalize
	if left == nil || right == nil || bytes.Compare(left, right) > 0 {
		return nil
	}

	return &IPRange{left: left, right: right}
}

func parseCIDRorMask(s string, sepIdx int) *IPRange {
	if sepIdx == len(s)-1 {
		return nil
	}

	addr := net.ParseIP(s[:sepIdx])
	if addr == nil {
		return nil
	}

	maskAsIP := net.ParseIP(s[sepIdx+1:])
	prefixLen, prefixLenErr := strconv.Atoi(s[sepIdx+1:])

	addrLen := len(addr)
	if addr.To4() != nil {
		addrLen = net.IPv4len // we should know this for mask length
	}

	var mask net.IPMask
	switch {
	case maskAsIP != nil: // maybe mask in IP form
		mask4 := maskAsIP.To4()
		if mask4 == nil || addrLen != net.IPv4len { // only for v4
			break
		}

		mask = net.IPMask(mask4)

		var addrLen int
		if prefixLen, addrLen = mask.Size(); prefixLen+addrLen == 0 {
			mask = nil // invalid mask
		}
	case prefixLenErr == nil: // maybe CIDR
		mask = net.CIDRMask(prefixLen, 8*addrLen)
	}

	if mask == nil {
		return nil
	}

	left := addr.Mask(mask)
	right := lastByMask(left, mask)

	// exclude broadcast addresses on generic masks
	if (addrLen == net.IPv4len && prefixLen < 31) ||
		(addrLen == net.IPv6len && prefixLen < 127) {
		// can just touch last bit
		left[len(left)-1] |= 1
		right[len(right)-1] &^= 1
	}

	return &IPRange{
		left:  left.To16(), // back to v6 form for canonical view
		right: right.To16(),
	}
}

func lastByMask(ip net.IP, mask net.IPMask) net.IP {
	ret := make(net.IP, len(ip))
	for i := range ip {
		ret[i] = ip[i] | ^mask[i]
	}

	return ret
}

func parseTwo(s string, sepIdx int) *IPRange {
	if sepIdx == len(s)-1 {
		return nil
	}

	left, right := net.ParseIP(s[:sepIdx]), net.ParseIP(s[sepIdx+1:])
	if left4, right4 := left.To4(), right.To4(); left4 == nil && right4 != nil || left4 != nil && right4 == nil {
		return nil // must be from same family
	}
	if left == nil || right == nil || bytes.Compare(left, right) > 0 {
		return nil
	}

	// left and right must be from one family

	return &IPRange{left: left, right: right}
}

// Contains tells if given IP is in range.
func (r *IPRange) Contains(ip net.IP) bool {
	ip = ip.To16() // normalize
	return bytes.Compare(ip, r.left) >= 0 && bytes.Compare(ip, r.right) <= 0
}

// UnmarshalText implements encoding.TextUnmarshaller.
func (r *IPRange) UnmarshalText(in []byte) error {
	parsed, err := ParseIPRange(string(in))
	if err != nil {
		return err
	}

	*r = *parsed
	return nil
}

func (r *IPRange) String() string {
	return fmt.Sprintf("%s-%s", r.left, r.right)
}
