package nspeed

import (
	"errors"
	"net"
	"strings"
)

// ResolveHostAddress resolves host as a dns name or an ip address
// works with ipv6%zone link-local address
func ResolveHostAddress(host string) ([]net.IPAddr, error) {
	var ipaddrs []net.IPAddr

	// special case, "ipv6%zone"
	hi := strings.SplitN(host, "%", 2)
	if len(hi) == 2 {
		ip := net.ParseIP(hi[0])
		if ip == nil {
			return nil, errors.New("bad IP format")
		}
		return append(ipaddrs, net.IPAddr{IP: ip, Zone: hi[1]}), nil
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if ip == nil {
			continue
		}
		ipaddr := net.IPAddr{IP: ip}
		ipaddrs = append(ipaddrs, ipaddr)
	}
	return ipaddrs, nil
}

// ResolveInterfaceHostAddress returns the IP address(es) of 'host' by
// resolving 'host' as, in that order,: interface name, dns name, ip address.
// if 'host' is a valid interface name, all IPs of that interface are returned, ipv6 link-local(s) are always last in the result.
// if 'host' is a not valid interface, 'host' is treated as a dns name and resolved as such.
func ResolveInterfaceHostAddress(host string) ([]net.IPAddr, error) {

	itf, err := net.InterfaceByName(host)

	// this is not a valid interface name, do dns
	if err != nil {
		return ResolveHostAddress(host)
	}

	// fails if  interface is not up
	if itf.Flags&net.FlagUp == 0 {
		return nil, errors.New("interface is down: " + itf.Name)
	}

	// get all the addresses of the interface
	addrs, err := itf.Addrs()
	if err != nil {
		return nil, err
	}

	// this is mess: we sort the interface addresses by IP version
	// IPv6 first (but we don't distinguish between GUA and ULA)
	// then IPv4
	// then link-local
	// this is temporary we will use SO_BINDTODEVICE later.
	var ipaddrs4 []net.IPAddr
	var ipaddrs6 []net.IPAddr
	var ipaddrs []net.IPAddr
	for _, addr := range addrs {
		ipn, ok := addr.(*net.IPNet)
		if !ok {
			//log.Error("not IPAddr %s\n", addr.String())
			continue
		}
		if ipn.IP == nil {
			continue
		}

		// convert IPNet to IPAddr
		ipa := net.IPAddr{IP: ipn.IP}
		// if link-local
		if ipa.IP.IsLinkLocalUnicast() {
			if ipa.IP.To4() == nil {
				ipa.Zone = itf.Name // todo: not sure this works on all platform, might use itf.Index
				ipaddrs = append([]net.IPAddr{ipa}, ipaddrs...)
			} else {
				ipaddrs = append(ipaddrs, ipa)
			}
		} else {
			if ipa.IP.To4() == nil {
				ipaddrs6 = append(ipaddrs6, ipa)
			} else {
				ipaddrs4 = append(ipaddrs4, ipa)
			}
		}
	}
	return append(append(ipaddrs6, ipaddrs4...), ipaddrs...), nil
}

// FilterAddresses
// filters a list of IP addresses based on IP version (0 = all or 4 or 6)
func FilterAddresses(addrs []net.IPAddr, ipversion int) []net.IPAddr {
	var fipaddrs []net.IPAddr

	if ipversion == 0 {
		return addrs
	}

	// bad ipversion -> return nothing
	if ipversion != 4 && ipversion != 6 {
		return nil
	}

	for _, addr := range addrs {
		if addr.IP.To4() == nil && ipversion == 6 {
			fipaddrs = append(fipaddrs, addr)
			continue
		}
		if addr.IP.To4() != nil && ipversion == 4 {
			fipaddrs = append(fipaddrs, addr)
			continue
		}
	}
	return fipaddrs
}

func HTTPVersionFlag(version int, h2c bool) string {
	r := ""
	if h2c {
		r = "-h2c "
	}
	if version != 0 {
		return r + "-http" + HTTPVersionStringer(version) + " "
	}
	return ""
}

func HTTPVersionStringer(version int) string {
	if version == 1 {
		return "1.1"
	}
	if version == 2 {
		return "2"
	}
	if version == 3 {
		return "3"
	}
	return "any"
}
