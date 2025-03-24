package network

import (
	"context"
	"fmt"
	"net"
	"time"

	"golang.org/x/net/quic"
)

// wip. x/net/quic isn't yet production ready
// quicStreamAsConn (temporary name) is a quic.Stream which satisfies the net.Conn interface
type quicStreamAsConn struct {
	*quic.Stream
	c           *quic.Conn
	readCtx     context.Context
	writeCtx    context.Context
	readCancel  context.CancelFunc
	writeCancel context.CancelFunc
}

// we can enforces here that quicStreamAsConn implements net.Conn
// var _ net.Conn = (*quicStreamAsConn)(nil)
// but NewQuicStreamAsConn does it already

// NewQuicStreamAsConn exposes a net.Conn interface on a quic.Stream
func NewQuicStreamAsConn(stream *quic.Stream, qconn *quic.Conn) net.Conn {
	// should we refuse nil args?
	return &quicStreamAsConn{
		Stream: stream,
		c:      qconn,
	}
}

// type Conn interface {
// 	Read(b []byte) (n int, err error) - implemented in embedded quic.Stream
// 	Write(b []byte) (n int, err error) - implemented in embedded quic.Stream
// 	Close() error - implemented in embedded quic.Stream but do more here too
// 	LocalAddr() Addr - implemented here
// 	RemoteAddr() Addr - implemented here
// 	SetDeadline(t time.Time) - error implemented here
// 	SetReadDeadline(t time.Time) - error implemented here
// 	SetWriteDeadline(t time.Time) -  error implemented here
// }

func (str *quicStreamAsConn) Close() error {
	if str.Stream == nil {
		return fmt.Errorf("no stream")
	}
	if str.readCancel != nil {
		str.readCancel()
	}
	if str.writeCancel != nil {
		str.writeCancel()
	}
	err := str.Stream.Close()
	return err
}

func (str *quicStreamAsConn) SetDeadline(t time.Time) error {
	if str.Stream == nil {
		return fmt.Errorf("SetDeadline on nil stream")
	}
	err := str.SetReadDeadline(t)
	if err != nil {
		return err
	}
	return str.SetWriteDeadline(t)
}

func (str *quicStreamAsConn) SetReadDeadline(t time.Time) error {
	if str.Stream == nil {
		return fmt.Errorf("SetReadDeadline on nil stream")
	}
	if str.writeCancel != nil {
		return fmt.Errorf("read deadline already set")
	}
	str.readCtx, str.readCancel = context.WithDeadline(context.Background(), t)
	str.Stream.SetReadContext(str.readCtx)
	return nil
}

func (str *quicStreamAsConn) SetWriteDeadline(t time.Time) error {
	if str.Stream == nil {
		return fmt.Errorf("SetWriteDeadline on nil stream")
	}
	if str.writeCancel != nil {
		return fmt.Errorf("write deadline already set")
	}
	str.writeCtx, str.writeCancel = context.WithDeadline(context.Background(), t)
	str.Stream.SetWriteContext(str.writeCtx)
	return nil
}

func (str *quicStreamAsConn) LocalAddr() net.Addr {
	if str.c == nil {
		return &net.UDPAddr{}
	}
	// nyi in std lib, use x/net fork. see https://github.com/golang/go/issues/70138
	return net.UDPAddrFromAddrPort(str.c.LocalAddr())
}

func (str *quicStreamAsConn) RemoteAddr() net.Addr {
	if str.c == nil {
		return &net.UDPAddr{}
	}
	// nyi in std lib, use x/net fork. see https://github.com/golang/go/issues/70138
	return net.UDPAddrFromAddrPort(str.c.RemoteAddr())
}

// end of net.Conn Interface

func (str *quicStreamAsConn) Flush() error {
	if str.Stream == nil {
		return fmt.Errorf("no stream to flush")
	}
	str.Stream.Flush()
	return nil
}
