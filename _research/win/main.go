package main

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
	"nspeed.app/webview"
)

func main() {

	// its, _ := winipcfg.GetAdaptersAddresses(0, winipcfg.GAAFlagSkipAnycast)

	// m := make(map[uint32]*winipcfg.IPAdapterAddresses)
	// fmt.Println("GetAdaptersAddresses:")
	// for idx, it := range its {
	// 	if it.OperStatus == winipcfg.IfOperStatusUp {
	// 		fmt.Printf("%d: %d %d - %s - %d\n", idx, it.IfIndex, it.IPv6IfIndex, it.FriendlyName(), it.OperStatus)
	// 		m[it.IfIndex] = it
	// 	}
	// }

	// fmt.Println("GetIfTable2Ex:")
	// m2, _ := winipcfg.GetIfTable2Ex(winipcfg.MibIfEntryNormal)
	// for idx, it := range m2 {
	// 	if _, ok := m[it.InterfaceIndex]; ok {
	// 		fmt.Printf("%d: %d %s %d %d\n", idx, it.InterfaceIndex, it.Alias(), it.InOctets, it.OutOctets)
	// 		//spew.Dump(it)
	// 	}
	// }

	ifaces, err := net.Interfaces()
	var itfs []net.Interface
	for _, itf := range ifaces {
		if itf.Flags&net.FlagUp != 0 {
			itfs = append(itfs, itf)
		}
	}
	if err != nil {
		fmt.Printf("localAddresses: %+v\n", err)
		return
	}
	rxs := webview.GetNetStats(itfs, "rx")
	txs := webview.GetNetStats(itfs, "tx")

	for i, it := range itfs {
		fmt.Printf("interface %d %s: rx=%f tx=%f \n", it.Index, it.Name, rxs[i], txs[i])
	}

	os.Exit(0)

	fmt.Println("GetIPForwardTable2 IPv4 :")
	rt4, _ := winipcfg.GetIPForwardTable2(windows.AF_INET)
	for _, r := range rt4 {
		fmt.Printf("%s\n", r.DestinationPrefix.IPNet())
		//spew.Dump(r)
	}

	fmt.Println("GetIPForwardTable2 IPv6 :")
	rt6, _ := winipcfg.GetIPForwardTable2(windows.AF_INET6)
	for _, r := range rt6 {
		fmt.Printf("%s\n", r.DestinationPrefix.IPNet())
		//spew.Dump(r)
	}

}
