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
		fmt.Printf("net.LookupHost(%s)=", s)
		for _, h := range addrs {
			a, err := netip.ParseAddr(h)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf(" %s ", a)
		}
		fmt.Printf("\n")

		udpAddr, err := net.ResolveUDPAddr("udp", s+":80")
		if err == nil {
			fmt.Printf("net.ResolveUDPAddr('udp', %s:80): %s\n", s, udpAddr)
		} else {
			fmt.Println("err:", err)
		}
		// resolv with ResolveUDPAddr
		udpAddr, err = net.ResolveUDPAddr("udp6", s+":80")
		if err == nil {
			fmt.Printf("net.ResolveUDPAddr('udp6', %s:80): %s\n", s, udpAddr)
		} else {
			fmt.Println("err:", err)
		}
	}
}
