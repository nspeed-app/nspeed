// Copyright (c) Jean-Francois Giorgi & AUTHORS
// parts of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause

package network

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
		err = fmt.Errorf("parse destination ip error: %w", err)
		return
	}
	iface, g, s, err := router.Route(ap.AsSlice())
	if err != nil {
		err = fmt.Errorf("router.Route error: %w", err)
		return
	}
	gw, _ = netip.AddrFromSlice(g)
	src, _ = netip.AddrFromSlice(s)
	return
}

// GetRouteFromAddrPort is the same as GetRoute but argument is in host:port format
func GetRouteFromAddrPort(destination string) (iface *net.Interface, gw netip.Addr, src netip.Addr, err error) {
	ip, _, err := net.SplitHostPort(destination)
	if err != nil {
		err = fmt.Errorf("split host port error: %w", err)
		return
	}
	return GetRoute(ip)
}
