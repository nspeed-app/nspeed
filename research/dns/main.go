package main

import (
	"fmt"
	"net"
	"os"
)

func main() {
	for _, s := range os.Args[1:] {
		addrs, _ := net.LookupHost(s)
		fmt.Println(s, "->", addrs)
	}
}
