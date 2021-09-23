package main

import (
	"fmt"

	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

func main() {

	its, _ := winipcfg.GetAdaptersAddresses(0, winipcfg.GAAFlagSkipAnycast)

	for idx, it := range its {
		if it.OperStatus == winipcfg.IfOperStatusUp {
			fmt.Printf("%d: %d - %s - %d\n", idx, it.IfIndex, it.FriendlyName(), it.OperStatus)
		}
	}

	m2, _ := winipcfg.GetIfTable2Ex(winipcfg.MibIfEntryNormal)
	for idx, it := range m2 {
		if it.AdminStatus == winipcfg.NetIfAdminStatusUp {
			fmt.Printf("%d: %d %s \n", idx, it.InterfaceIndex, it.Alias(), it.)
		}
	}

}
