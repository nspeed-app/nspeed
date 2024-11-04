package utils

import (
	"context"
	"fmt"
	"net"
	"time"

	"golang.org/x/net/quic"
)

// QuicStreamAsConn (temporary name) is a quic.Stream which satisfies the net.Conn interface
type QuicStreamAsConn struct {
	*quic.Stream
	c           *quic.Conn
	readCtx     context.Context
	writeCtx    context.Context
	readCancel  context.CancelFunc
	writeCancel context.CancelFunc
}

// this enforces here that quicStreamAsConn implements net.Conn
// var _ net.Conn = (*QuicStreamAsConn)(nil)

// NewQuicStreamAsConn exposes the net.Conn interface on a quic.Stream
func NewQuicStreamAsConn(stream *quic.Stream, qconn *quic.Conn) net.Conn {
	return &QuicStreamAsConn{
		Stream: stream,
		c:      qconn,
	}
}

// type Conn interface {
// 	Read(b []byte) (n int, err error) - embedded quic.Stream
// 	Write(b []byte) (n int, err error) - embedded quic.Stream
// 	Close() error - embedded quic.Stream but do more stuff
// 	LocalAddr() Addr - implement here
// 	RemoteAddr() Addr - implement here
// 	SetDeadline(t time.Time) - error implement here
// 	SetReadDeadline(t time.Time) - error implement here
// 	SetWriteDeadline(t time.Time) -  error implement here
// }

func (str *QuicStreamAsConn) Close() error {
	if str.Stream == nil {
		return fmt.Errorf("nothing to close")
	}
	if str.readCancel != nil {
		str.readCancel()
	}
	if str.writeCancel != nil {
		str.writeCancel()
	}
	err := str.Stream.Close()
	return err
	// todo: add an optionnal way to close the conn when we close its last stream ?
}
func (str *QuicStreamAsConn) SetDeadline(t time.Time) error {
	if str.Stream == nil {
		return fmt.Errorf("SetDeadline on nil stream")
	}
	err := str.SetReadDeadline(t)
	if err != nil {
		return err
	}
	return str.SetWriteDeadline(t)
}

func (str *QuicStreamAsConn) SetReadDeadline(t time.Time) error {
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

func (str *QuicStreamAsConn) SetWriteDeadline(t time.Time) error {
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

func (str *QuicStreamAsConn) LocalAddr() net.Addr {
	if str.c == nil {
		return &net.UDPAddr{}
	}
	// nyi in std lib, use x/net fork. see https://github.com/golang/go/issues/70138
	return net.UDPAddrFromAddrPort(str.c.LocalAddr())
}

func (str *QuicStreamAsConn) RemoteAddr() net.Addr {
	if str.c == nil {
		return &net.UDPAddr{}
	}
	// nyi in std lib, use x/net fork. see https://github.com/golang/go/issues/70138
	return net.UDPAddrFromAddrPort(str.c.RemoteAddr())
}

// end of net.Conn Interface

func (str *QuicStreamAsConn) Flush() error {
	if str.Stream == nil {
		return fmt.Errorf("no stream to flush")
	}
	str.Stream.Flush()
	return nil
}
