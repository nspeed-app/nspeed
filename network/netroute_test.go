// Copyright (c) Jean-Francois Giorgi & AUTHORS
// parts of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause

package network

import (
	"testing"
)

func TestGetRoute(t *testing.T) {
	// we can't test much here. So we test localhost for IPv4 and IPv6
	_, _, _, err := GetRoute("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = GetRoute("::1") // investigate: some ci/cd runners may not have ipv6?
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetRouteFromAddrPort(t *testing.T) {
	// we can't test much here. So we test localhost for IPv4 and IPv6
	_, _, _, err := GetRouteFromAddrPort("127.0.0.1:80")
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = GetRouteFromAddrPort("[::1]:0")
	if err != nil {
		t.Fatal(err)
	}
}
