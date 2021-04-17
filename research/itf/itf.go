package main

import (
	"fmt"
	"net"
	"os"

	"nspeed.app/nspeed"
)

func localAddresses() {
	ifaces, err := net.Interfaces()
	if err != nil {
		fmt.Print(fmt.Errorf("localAddresses: %+v\n", err.Error()))
		return
	}
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			fmt.Print(fmt.Errorf("localAddresses: %+v\n", err.Error()))
			continue
		}
		for _, a := range addrs {
			switch v := a.(type) {
			case *net.IPAddr:
				fmt.Printf("%v : %s (%s)\n", i.Name, v, v.IP.DefaultMask())

			case *net.IPNet:
				fmt.Printf("%v : %s [%v/%v]\n", i.Name, v, v.IP, v.Mask)
			}

		}
	}
}

func main() {
	localAddresses()
	fmt.Printf("calling for %s\n", os.Args[1:])
	for _, a := range os.Args[1:] {
		ipaddrs, err := nspeed.ResolveInterfaceHostAddress(a)
		if err != nil {
			fmt.Printf("%s\n", err)
		}
		for _, addr := range ipaddrs {
			fmt.Printf("%s : %s\n", a, addr.String())
		}
	}
}
