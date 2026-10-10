package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/jsimonetti/rtnetlink/rtnl"
)

// get route to destination
// jsimonetti version of rtnetlink
// see also with https://github.com/vishvananda/netlink

func main() {
	conn, err := rtnl.Dial(nil)
	if err != nil {
		log.Fatal("can't establish netlink connection: ", err)
	}
	defer conn.Close()
	// use conn for your calls

	route, err := conn.RouteGet(net.ParseIP(os.Args[1]))

	if err != nil {
		log.Fatal("can't get route: ", err)
	}
	addr, err := route.Interface.Addrs()
	if err != nil {
		log.Fatal("can't get address: ", err)
	}

	fmt.Printf("route %s: from %s, dev %s, gateway %s\n", os.Args[1], addr, route.Interface.Name, route.Gateway)
}
