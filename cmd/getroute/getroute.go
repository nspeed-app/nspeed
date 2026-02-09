// Copyright (c) Jean-Francois Giorgi & AUTHORS
// parts of nspeed.app
// parts are from https://go.dev/
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"flag"
	"fmt"
	"os"

	"nspeed.app/nspeed/network"
)

// a sample program to demo parts of the nspeed.app/network package
// it's equivalent to "ip route get target" command line on Linux but
// target can be a hostname/fqdn
// multiple targets can be used
// for instance:
//
//	getroute dns.google one.one.one.one
func main() {

	var v4 = flag.Bool("4", false, `use IPv4`)
	var v6 = flag.Bool("6", false, `use IPv6`)

	flag.Usage = func() {
		name := "getroute"
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n\n", name)
		fmt.Fprintf(flag.CommandLine.Output(), "  %s [options] target ...\n\n", name)
		fmt.Fprintf(flag.CommandLine.Output(), "target can be an IP address or a DNS name\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Available options:\n\n")
		flag.PrintDefaults()
		fmt.Fprintf(flag.CommandLine.Output(), "\n")
	}
	flag.Parse()

	var ipVersion network.IPVersion
	if *v4 && *v6 {
		fmt.Println("cannot specify both IP version at the same time")
		os.Exit(1)
	}
	if *v4 {
		ipVersion = 4
	}
	if *v6 {
		ipVersion = 6
	}

	if flag.NArg() == 0 {
		fmt.Println("no target")
		flag.Usage()
		os.Exit(1)
	}

	// loop thru targets
	for _, v := range flag.Args() {
		addr, err := network.ResolveHostAddress(v)
		if err != nil {
			fmt.Println("resolve error:", err, "for ", v)
			continue
		}
		// filter addresses based on ip version
		addrf := network.FilterAddresses(addr, ipVersion)
		if len(addrf) == 0 {
			fmt.Println("no matching ip found for", v)
			continue
		}
		// loop thru addresses of the target
		for _, a := range addrf {
			fmt.Printf("%s = %s:\n", v, a.String())
			iface, gw, src, err := network.GetRoute(a.String())
			if err != nil {
				fmt.Println("GetRoute error:", err)
				continue
			}
			fmt.Println("  gateway/next-hop: ", gw)
			fmt.Println("  source address: ", src)
			fmt.Printf("  interface: %s (%d)\n", iface.Name, iface.Index)
		}
	}
}
