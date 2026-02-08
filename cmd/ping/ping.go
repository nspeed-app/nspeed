// Copyright (c) Jean-Francois Giorgi & AUTHORS
// parts of nspeed.app
// parts are from https://go.dev/
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"flag"
	"fmt"
	"os"

	"nspeed.app/nspeed/ping"
)

func main() {

	var m = flag.Int("m", 0, `Set the max time-to-live (max number of hops) used in outgoing probe packets (default is 0 = OS default)`)
	var s = flag.Uint("s", 0, `Set the packet size to use (default is 0 = no timeout)`)
	var w = flag.Duration("w", 0, `Set timeout (default none)`)
	var v4 = flag.Bool("4", false, `use IPv4`)
	var v6 = flag.Bool("6", false, `use IPv6`)

	flag.Parse()

	options := ping.PingOptions{HopLimit: *m, Timeout: *w}
	// additionnal flags checks
	if *v4 && *v6 {
		fmt.Println("cannot specify both ip version at the same time")
		os.Exit(1)
	}
	if *v4 {
		options.Version = 4
	}
	if *v6 {
		options.Version = 6
	}
	if *s > ping.PacketSizeMax {
		fmt.Println("size if too big, max is", ping.PacketSizeMax)
		os.Exit(1)
	}
	options.PacketSize = uint16(*s)

	if flag.NArg() == 0 {
		fmt.Println("no target")
		os.Exit(1)
	}
	for _, host := range flag.Args() {

		peer, ping, response, err := ping.Ping(host, options)
		if err != nil {
			fmt.Println("ping error:", err)
		} else {
			fmt.Println(host, "ip is", peer, "time: ", ping, "code:", response)
		}
	}
}
