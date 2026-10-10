package main

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
)

type EthrProtocol string

const TCP = "tcp"
const UDP = "udp"

func Tcp() string {
	return "tcp"
}

func Udp() string {
	return "UDP"
}

// os/plat specific:
func setSockOptInt(fd uintptr, level, opt, val int) (err error) {
	err = syscall.SetsockoptInt(int(fd), level, opt, val)
	if err != nil {
		fmt.Printf("Failed to set socket option (%v) to value (%v) during Dial. Error: %s", opt, val, err)
	}
	return
}
func SetTClass(fd uintptr, tos int) {
	setSockOptInt(fd, syscall.IPPROTO_IPV6, syscall.IPV6_TCLASS, tos)
}

// end of os/plat

func ethrSetTTL(fd uintptr, ttl int) {
	if ttl == 0 {
		return
	}
	if gIPVersion == ethrIPv4 {
		setSockOptInt(fd, syscall.IPPROTO_IP, syscall.IP_TTL, ttl)
	} else {
		setSockOptInt(fd, syscall.IPPROTO_IPV6, syscall.IPV6_UNICAST_HOPS, ttl)
	}
}

func ethrSetTOS(fd uintptr, tos int) {
	if tos == 0 {
		return
	}
	if gIPVersion == ethrIPv4 {
		setSockOptInt(fd, syscall.IPPROTO_IP, syscall.IP_TOS, tos)
	} else {
		SetTClass(fd, tos)
	}
}

func ethrDialEx(p EthrProtocol, dialAddr, localIP string, localPortNum uint16, ttl int, tos int) (conn net.Conn, err error) {
	localAddr := fmt.Sprintf("%v:%v", localIP, localPortNum)
	var la net.Addr
	network := Tcp()
	if p == TCP {
		la, err = net.ResolveTCPAddr(network, localAddr)
	} else if p == UDP {
		network = Udp()
		la, err = net.ResolveUDPAddr(network, localAddr)
	} else {
		fmt.Println("Only TCP or UDP are allowed in ethrDial")
		err = os.ErrInvalid
		return
	}
	if err != nil {
		fmt.Println("Unable to resolve TCP or UDP address. Error: %v", err)
		return
	}
	dialer := &net.Dialer{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				ethrSetTTL(fd, ttl)
				ethrSetTOS(fd, tos)
			})
		},
	}
	dialer.LocalAddr = la
	dialer.Timeout = time.Second
	conn, err = dialer.Dial(network, dialAddr)
	if err != nil {
		ui.printDbg("ethrTCPDial Error: %v", err)
	} else {
		tcpconn, ok := conn.(*net.TCPConn)
		if ok {
			tcpconn.SetLinger(0)
		}
		udpconn, ok := conn.(*net.UDPConn)
		if ok {
			err = udpconn.SetWriteBuffer(4 * 1024 * 1024)
			if err != nil {
				ui.printDbg("Failed to set ReadBuffer on UDP socket: %v", err)
			}
		}
	}
	return
}
