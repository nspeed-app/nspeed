package utils

import (
	"fmt"
	"net"
	"net/netip"

	"github.com/google/gopacket/routing"
	"github.com/libp2p/go-netroute"
)

// RouteurError is not nil if there was an issue creating the internal router
var RouterError error

var router routing.Router

func init() {
	router, RouterError = netroute.New()
}

// GetRoute returns routing informations for a destination which must be a literal IP address (v4 or v6).
//
// The returned informations are:
//
//	iface: the net.Interface used to reach the destination
//	gw: the gateway address used (can be nul)
//	src: the source address used (can be nul)
func GetRoute(destination string) (iface *net.Interface, gw netip.Addr, src netip.Addr, err error) {
	if router == nil {
		err = fmt.Errorf("no router")
		return
	}
	if RouterError != nil {
		err = fmt.Errorf("router error: %w", RouterError)
		return
	}
	ap, err := netip.ParseAddr(destination)
	if err != nil {
		return
	}
	iface, g, s, err := router.Route(ap.AsSlice())
	if err != nil {
		return
	}
	gw, _ = netip.AddrFromSlice(g)
	src, _ = netip.AddrFromSlice(s)
	return
}
