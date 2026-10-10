//go:build linux

package main

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/sys/unix"
)

func main() {
	listener, err := net.Listen("tcp", ":0")
	check(err)

	fmt.Println("Listening on", listener.Addr())

	for {
		conn, err := listener.Accept()
		check(err)
		go func(conn *net.TCPConn) {
			defer conn.Close()
			info, err := tcpInfo(conn)
			check(err)
			rtt := time.Duration(info.Rtt) * time.Microsecond
			fmt.Println(rtt)
		}(conn.(*net.TCPConn))
	}
}

func tcpInfo(conn *net.TCPConn) (*unix.TCPInfo, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return nil, err
	}

	var info *unix.TCPInfo
	ctrlErr := raw.Control(func(fd uintptr) {
		info, err = unix.GetsockoptTCPInfo(int(fd), unix.IPPROTO_TCP, unix.TCP_INFO)
	})
	switch {
	case ctrlErr != nil:
		return nil, ctrlErr
	case err != nil:
		return nil, err
	}
	return info, nil
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
