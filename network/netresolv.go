// Copyright (c) Jean-Francois Giorgi & AUTHORS
// parts of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause

package network

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
)

// Resolve performs basic resolution and return the first address matching the IP version (0 = OS preference)
func Resolve(dest string, ipVersion int) (destAddr netip.Addr, err error) {
	addrs, err := net.LookupIP(dest)
	if err != nil {
		return
	}
	for _, addr := range addrs {
		var ok bool
		destAddr, ok = netip.AddrFromSlice(addr)
		if !ok {
			err = fmt.Errorf("invalid IP length: %s", addr)
			return
		}
		if ipVersion == 0 {
			return
		}
		if ipVersion == 4 && destAddr.Is4() {
			return
		}
		if ipVersion == 6 && destAddr.Is6() && !destAddr.Is4In6() {
			return
		}
	}
	err = errors.New("no address found")
	return
}
