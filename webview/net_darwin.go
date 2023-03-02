package webview

import (
	"bytes"
	"encoding/binary"
	"golang.org/x/sys/unix"
	"net"
)

func HasNetStats() error {
	return nil
}

func GetNetStats(interfaces []net.Interface, dir string) []float64 {

	var values []float64
	for _, iface := range interfaces {
		ifaceData, err := getIfaceData(iface.Index)
		value := float64(-1)
		if err == nil {
			if dir == "tx" {
				value = float64(ifaceData.Data.Obytes)
			}
			if dir == "rx" {
				value = float64(ifaceData.Data.Ibytes)
			}
		}
		values = append(values, value)
	}
	return values
}

func getIfaceData(index int) (*ifMsghdr2, error) {
	var data ifMsghdr2
	rawData, err := unix.SysctlRaw("net", unix.AF_ROUTE, 0, 0, unix.NET_RT_IFLIST2, index)
	if err != nil {
		return nil, err
	}
	err = binary.Read(bytes.NewReader(rawData), binary.LittleEndian, &data)
	return &data, err
}

type ifMsghdr2 struct {
	Msglen    uint16
	Version   uint8
	Type      uint8
	Addrs     int32
	Flags     int32
	Index     uint16
	_         [2]byte
	SndLen    int32
	SndMaxlen int32
	SndDrops  int32
	Timer     int32
	Data      ifData64
}

// https://github.com/apple/darwin-xnu/blob/main/bsd/net/if_var.h#L199-L231
type ifData64 struct {
	Type       uint8
	Typelen    uint8
	Physical   uint8
	Addrlen    uint8
	Hdrlen     uint8
	Recvquota  uint8
	Xmitquota  uint8
	Unused1    uint8
	Mtu        uint32
	Metric     uint32
	Baudrate   uint64
	Ipackets   uint64
	Ierrors    uint64
	Opackets   uint64
	Oerrors    uint64
	Collisions uint64
	Ibytes     uint64
	Obytes     uint64
	Imcasts    uint64
	Omcasts    uint64
	Iqdrops    uint64
	Noproto    uint64
	Recvtiming uint32
	Xmittiming uint32
	Lastchange unix.Timeval32
}
