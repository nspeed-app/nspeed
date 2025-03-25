package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// SetDNSServer specifies a custom DNS server (globally).
// The parameter must be a literal IP address or literal IP:port combination.
// if no port, 53 will be used.

func SetDNSServer(address string) error {
	addrPort, err := netip.ParseAddrPort(address)
	if err != nil {
		addr, err2 := netip.ParseAddr(address)
		if err2 != nil {
			return err2
		}
		addrPort = netip.AddrPortFrom(addr, 53)
	}
	net.DefaultResolver.PreferGo = true
	net.DefaultResolver.Dial = func(context context.Context, network, address string) (net.Conn, error) {
		var d net.Dialer
		conn, err := d.DialContext(context, "udp", addrPort.String())
		if err != nil {
			return nil, err
		}
		return conn, nil
	}
	return nil
}

// ParseAddressWithOptionnalPort tries to split a trailing :port part of hostport
//
//	host can be a literal address or a hostname
//	A literal IPv6 address with a port must be enclosed in square brackets, as in "[::1]:80", "[::1%lo0]:80".
func ParseAddressWithOptionnalPort(hostport string) (string, int, error) {
	host, portString, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
		portString = "0"
	}
	// in case net.SplitHostPort
	if portString == "" {
		portString = "0"
	}
	port, err := strconv.ParseUint(portString, 10, 16)
	if err != nil {
		port = 0
	}
	return host, int(port), err
}

// ResolveHostAddress resolves host as a dns name or an ip address
// works with ipv6%zone link-local address
func ResolveHostAddress(host string) ([]net.IPAddr, error) {
	var ipaddrs []net.IPAddr

	// special case, 'localhost' we should always resolves our self
	// todo:
	//  get loopback interface addresses
	//
	// if host == "localhost" {
	// 	ips := []net.IPAddr{
	// 		{IP: net.IPv6loopback},
	// 		{IP: net.IPv4(127, 0, 0, 1)},
	// 	}
	// 	return ips, nil
	// }
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
	// this is temporary we will use SO_BINDTODEVICE later (on supported plateforms)
	var ipaddrs4 []net.IPAddr
	var ipaddrs6 []net.IPAddr
	var ipaddrs []net.IPAddr
	for _, addr := range addrs {
		ipn, ok := addr.(*net.IPNet)
		if !ok {
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

// InterfaceAddress returns the address of interface if parameter host is an interface name.
// If ipVersion is not 0, the corresponding version is selected.

func InterfaceAddress(host string, ipVersion IPVersion) (*net.IPAddr, error) {
	if host == "" {
		return nil, nil
	}
	la, err := ResolveInterfaceHostAddress(host)
	if err != nil {
		return nil, fmt.Errorf("bad local host address: %s (%w)", host, err)
	}

	fhostIPAddrs := FilterAddresses(la, ipVersion)

	if len(fhostIPAddrs) == 0 {
		return nil, fmt.Errorf("no valid candidate address found for %s", host)
	}

	return &fhostIPAddrs[0], nil
}

// FilterAddresses
// filters a list of IP addresses based on IP version (0 = all or 4 or 6)
func FilterAddresses(addrs []net.IPAddr, ipversion IPVersion) []net.IPAddr {
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

// parse URL , append default port
func ParseURL(u, scheme string) (*url.URL, error) {
	u1, err := url.ParseRequestURI(u)
	if err != nil || u1.Host == "" {
		//fmt.Println("error 1")
		u2, err := url.ParseRequestURI(scheme + "://" + u)
		if err != nil {
			//fmt.Println("error 2")
			return nil, err
		}
		return u2, nil
	}
	return u1, nil
}

// GetIntefaceNames returns the Name members of a net.Interface list
func GetIntefaceNames(interfaces []net.Interface) []string {
	var names = []string{}
	for _, it := range interfaces {
		names = append(names, it.Name)
	}
	return names
}

// GetNetInterfaces returns the net.Interface values of interface names parameter or
// all interface if no names (nil) - case sensitive
func GetNetInterfaces(names []string) ([]net.Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	ifts := ifaces
	if names != nil {
		ifts = nil
		for _, n := range names {
			found := false
			for _, it := range ifaces {
				if n == it.Name {
					ifts = append(ifts, it)
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("interface \"%s\" not found", n)
			}
		}
	}
	return ifts, nil
}
