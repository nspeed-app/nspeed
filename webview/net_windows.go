package webview

import (
	"net"

	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

func HasNetStats() error {
	return nil
}

type MibIfEntryLevel = winipcfg.MibIfEntryLevel
type MibIfRow2 = winipcfg.MibIfRow2

//sys	getIfEntry2Ex(level MibIfEntryLevel, row *MibIfRow2) (ret error) = iphlpapi.GetIfEntry2Ex

// The GetIfEntry2Ex function retrieves the specified level of information for the specified interface on the local computer.
// https://docs.microsoft.com/en-us/windows/win32/api/netioapi/nf-netioapi-getifentry2ex
// A pointer to a MIB_IF_ROW2 structure that, on successful return, receives information for an interface on the local computer.
// On input, the InterfaceLuid or the InterfaceIndex member of the MIB_IF_ROW2 must be set to the interface for which to retrieve information.
func GetIfEntry2Ex(ifIndex uint32, level MibIfEntryLevel) (*MibIfRow2, error) {
	tab := MibIfRow2{InterfaceIndex: ifIndex}

	err := getIfEntry2Ex(winipcfg.MibIfEntryNormal, &tab)
	if err != nil {
		return nil, err
	}
	return &tab, nil
}

// return rx/tx counters for interfaces - Linux version
func GetNetStats(itfs []net.Interface, dir string) []float64 {
	var values []float64
	for i := range itfs {
		info, err := GetIfEntry2Ex(uint32(itfs[i].Index), winipcfg.MibIfEntryNormal)
		if err != nil {
			panic("interface not found calling getIfEntry2Ex: " + itfs[i].Name)
		}
		if dir == "tx" {
			values = append(values, float64(info.OutOctets))
		}
		if dir == "rx" {
			values = append(values, float64(info.InOctets))
		}
	}
	return values
}
