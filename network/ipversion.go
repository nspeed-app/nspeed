package network

import "net/netip"

// 0,4,6 or -1 if no valid
type IPVersion int

func (v IPVersion) IsValid() bool {
	return v == 4 || v == 6 || v == 0
}

func (v IPVersion) String() string {
	switch v {
	case 4:
		return "IPv4"
	case 6:
		return "IPv6"
	case -1:
		return "invalid"
	default:
		return ""
	}
}

func (v IPVersion) NumericString() string {
	switch v {
	case 4:
		return "4"
	case 6:
		return "6"
	case -1:
		return "invalid"
	default:
		return ""
	}
}

func GetIPVersion(address netip.Addr) IPVersion {
	if address.Is4() {
		return 4
	}
	if address.Is6() {
		return 6
	}
	return 0
}

// AddIPVersionToNetwork adds the ipVersion to network.
// "udp", 4  -> "udp4"
func AddIPVersionToNetwork(network string, ipVersion IPVersion) string {
	if ipVersion == 0 {
		return network
	}
	return network + ipVersion.NumericString()
}
