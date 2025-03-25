// Copyright (c) Jean-Francois Giorgi & AUTHORS
// parts of nspeed.app
// parts are from https://go.dev/
// SPDX-License-Identifier: BSD-3-Clause

// Package ping allows to perform network 'ping' operations. See Ping.Ping(...).
package ping

import (
	"fmt"
	"math"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
	"nspeed.app/iana"
	"nspeed.app/network"
)

type PingOptions struct {
	HopLimit   int           // max TTL
	Version    int           // IP version: 0,4 or 6
	PacketSize uint16        // size of ping packet up to PacketSizeMax
	Timeout    time.Duration // timeout , 0 = no timeout
}

const PacketSizeMax = math.MaxUint16 // 64KB

// PingResponse is the ping response, most common ICMP responses
type PingResponse int

// this is a 'merge' of most common IPv6 and IPv4 ICMP response messages
const (
	PingResponseNotHandled             PingResponse = 0 // all others
	PingResponseDestinationUnreachable PingResponse = 1 // Destination Unreachable
	PingResponsePacketTooBig           PingResponse = 2 // Packet Too Big
	PingResponseTimeExceeded           PingResponse = 3 // Time Exceeded
	PingResponseEchoReply              PingResponse = 4 // Echo Reply
)

var pingResponseNames = map[PingResponse]string{
	PingResponseDestinationUnreachable: "destination unreachable",
	PingResponsePacketTooBig:           "packet too big",
	PingResponseTimeExceeded:           "time exceeded",
	PingResponseEchoReply:              "ok",
	PingResponseNotHandled:             "unknown",
}

func (p PingResponse) String() string {
	n, ok := pingResponseNames[p]
	if ok {
		return n
	}
	return "hidden ping response"
}

// single shared buffer for sending.
var sendBuffer []byte

func allocBuffer() {
	sync.OnceFunc(func() {
		//fmt.Println("ALLOCATING")
		sendBuffer = make([]byte, PacketSizeMax)
	})()
}

var count atomic.Int32

// Ping performs a single ICMP echo request to destination.
// On Unix platforms requires root or cap_net_raw capability.
//
// This is not thread safe even at the OS level safe (a concurrent ping or mtr will impact the result)
// the Seq doesn't seem to be used by the Go x/net/icmp package
func Ping(destination string, options PingOptions) (peer net.Addr, ping time.Duration, response PingResponse, err error) {

	destAddr, err := network.Resolve(destination, options.Version)
	if err != nil {
		return
	}
	isIPv4 := destAddr.Is4() || destAddr.Is4In6()

	// this should never arise but in case:
	if isIPv4 && options.Version == 6 {
		err = fmt.Errorf("IP version mismatch")
		return
	}

	network := "ip6:ipv6-icmp"
	laddr := "::"
	var itype icmp.Type = ipv6.ICMPTypeEchoRequest
	protonumber := iana.ProtocolIPv6ICMP
	if isIPv4 {
		network = "ip4:icmp"
		laddr = "0.0.0.0"
		itype = ipv4.ICMPTypeEcho
		protonumber = iana.ProtocolICMP
	}

	c, err := icmp.ListenPacket(network, laddr)
	if err != nil {
		err = fmt.Errorf("listen packet error: %w", err)
		return
	}
	defer c.Close()

	// TTL
	ttl := options.HopLimit

	if ttl > 0 && isIPv4 {
		err = c.IPv4PacketConn().SetTTL(ttl)
	}
	if ttl > 0 && !isIPv4 {
		err = c.IPv6PacketConn().SetHopLimit(ttl)
	}
	if err != nil {
		err = fmt.Errorf("error setting TTL: %w", err)
		return
	}

	allocBuffer()
	size := 0
	if options.PacketSize > 0 {
		size = int(options.PacketSize) - 1
	}
	wm := icmp.Message{
		Type: itype, Code: 0,
		Body: &icmp.Echo{
			ID:   os.Getpid() & 0xffff,
			Seq:  int(count.Add(1)),
			Data: sendBuffer[:size],
		},
	}
	wb, err := wm.Marshal(nil)
	if err != nil {
		err = fmt.Errorf("Message Marshal error: %w", err)
		return
	}

	if options.Timeout > 0 {
		_ = c.SetWriteDeadline(time.Now().Add(options.Timeout))
	}

	rb := make([]byte, 1500)
	start := time.Now()
	if _, err = c.WriteTo(wb, &net.IPAddr{IP: destAddr.AsSlice()}); err != nil {
		err = fmt.Errorf("WriteTo error: %w", err)
		return
	}

	if options.Timeout > 0 {
		_ = c.SetReadDeadline(time.Now().Add(options.Timeout))
	}
	n, peer, err := c.ReadFrom(rb)
	if err != nil {
		err = fmt.Errorf("read error: %w", err)
		return
	}

	ping = time.Since(start)
	rm, err := icmp.ParseMessage(protonumber, rb[:n])
	if err != nil {
		err = fmt.Errorf("ParseMessage error: %w", err)
		return
	}
	switch rm.Type {
	case ipv6.ICMPTypeEchoReply, ipv4.ICMPTypeEchoReply:
		response = PingResponseEchoReply
	case ipv6.ICMPTypePacketTooBig:
		response = PingResponsePacketTooBig
	case ipv6.ICMPTypeDestinationUnreachable, ipv4.ICMPTypeDestinationUnreachable:
		response = PingResponseDestinationUnreachable
	case ipv6.ICMPTypeTimeExceeded, ipv4.ICMPTypeTimeExceeded:
		response = PingResponseTimeExceeded
	default: // eventually handle more
		response = PingResponseNotHandled
	}
	return
}
