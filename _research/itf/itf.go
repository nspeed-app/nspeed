package main

import (
	"fmt"
	"net"
	"os"

	"nspeed.app/network"
)

func localAddresses() {
	ifaces, err := net.Interfaces()
	if err != nil {
		fmt.Printf("localAddresses: %+v\n", err)
		return
	}
	for _, i := range ifaces {
		fmt.Printf("interface %d = %s:\n", i.Index, i.Name)
		addrs, err := i.Addrs()
		if err != nil {
			fmt.Printf("localAddresses: %+v\n", err)
			continue
		}
		for _, a := range addrs {
			switch v := a.(type) {
			case *net.IPAddr:
				fmt.Printf("  %v : %s (%s)\n", "i.Name", v, v.IP.DefaultMask())

			case *net.IPNet:
				fmt.Printf("  %v : %s [%v/%v]\n", i.Name, v, v.IP, v.Mask)
			}

		}
	}
}

func main() {
	if len(os.Args) == 1 {
		localAddresses()
		return
	}
	for _, a := range os.Args[1:] {
		fmt.Printf("calling ResolveInterfaceHostAddress for %s\n", a)
		ipaddrs, err := network.ResolveInterfaceHostAddress(a)
		if err != nil {
			fmt.Printf("%s\n", err)
		}
		for _, addr := range ipaddrs {
			fmt.Printf("%s : %s\n", a, addr.String())
		}
	}
}
