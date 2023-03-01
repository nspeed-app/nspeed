package main

import (
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
)

func main() {
	for _, s := range os.Args[1:] {
		addrs, _ := net.LookupHost(s)
		ns, _ := net.LookupNS(s)
		fmt.Printf("%s:\n", s)
		for _, h := range addrs {
			a, err := netip.ParseAddr(h)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf(" %s ", a)
		}
		fmt.Printf("%s: %+v\n", s, ns)
	}
}
