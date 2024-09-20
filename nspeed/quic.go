package nspeed

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/quic"
	"nspeed.app/metric"
)

func quicClientInit(j *JobClient) (*quic.Config, *quic.Endpoint) {
	quicConf := &quic.Config{}
	endp := &quic.Endpoint{}

	// todo tracing

	// since we use Dialcontext we need to set a proper local *net.UDPAddr for LocalAddr
	if j.localAddr != nil {
		j.dialer.LocalAddr = &net.UDPAddr{
			IP:   j.localAddr.IP,
			Zone: j.localAddr.Zone,
		}
	}
	return quicConf, endp
}

type quicStreamAsConn struct {
	*quic.Stream
	e   *quic.Endpoint
	c   *quic.Conn
	who string // remove or replace with a Logger
}

// this enforces here that quicStreamAsConn implements net.Conn
var _ net.Conn = (*quicStreamAsConn)(nil)

// type Conn interface {
// 	Read(b []byte) (n int, err error) - ok
// 	Write(b []byte) (n int, err error) - ok
// 	Close() error - ok
//
// 	LocalAddr() Addr
// 	RemoteAddr() Addr
// 	SetDeadline(t time.Time) error
// 	SetReadDeadline(t time.Time) error
// 	SetWriteDeadline(t time.Time) error
// }

func (str quicStreamAsConn) Read(b []byte) (n int, err error) {
	n, err = str.Stream.Read(b)
	fmt.Println(str.who, "READING", n, err)
	return
}

func (str quicStreamAsConn) Close() error {
	if str.Stream == nil {
		return fmt.Errorf("nothign to close")
	}
	str.Stream.Close()
	str.c.Close()
	return str.e.Close(context.Background())
}
func (str quicStreamAsConn) SetDeadline(t time.Time) error {
	if str.Stream == nil {
		return fmt.Errorf("SetDeadline on nil stream")
	}
	ctx, close := context.WithDeadline(context.Background(), t)
	defer close()
	str.Stream.SetReadContext(ctx)
	str.Stream.SetWriteContext(ctx)
	return nil
}

func (str quicStreamAsConn) SetReadDeadline(t time.Time) error {
	if str.Stream == nil {
		return fmt.Errorf("SetReadDeadline on nil stream")
	}
	ctx, close := context.WithDeadline(context.Background(), t)
	defer close()
	str.Stream.SetReadContext(ctx)
	return nil
}

func (str quicStreamAsConn) SetWriteDeadline(t time.Time) error {
	fmt.Println(str.who, "SETTING WRITE DEADLINE of %d", t)
	if str.Stream == nil {
		return fmt.Errorf("SetWriteDeadline on nil stream")
	}
	ctx, close := context.WithDeadline(context.Background(), t)
	defer close()
	str.Stream.SetWriteContext(ctx)
	return nil
}

func (str quicStreamAsConn) LocalAddr() net.Addr {
	if str.e == nil {
		return &net.UDPAddr{}
	}
	return net.UDPAddrFromAddrPort(str.e.LocalAddr())
}

func (str quicStreamAsConn) RemoteAddr() net.Addr {
	if str.c == nil {
		return &net.UDPAddr{}
	}
	return net.UDPAddrFromAddrPort(str.c.RemoteAddr())
}

func (str quicStreamAsConn) Flush() error {
	if str.Stream == nil {
		return fmt.Errorf("no stream to flush")
	}
	str.Stream.Flush()
	return nil
}

// SERVER

func (j *L4Server) ServeQUIC(ctx context.Context, network string, wg *sync.WaitGroup) (*metric.Metric, error) {
	// this avoid calling wg.Done() twice
	done := false
	defer func() {
		if !done {
			wg.Done()
		}
	}()

	if j.tlsConfig == nil {
		return nil, fmt.Errorf("ServeQUIC: no TLS config")
	}

	if j.tlsConfig.MinVersion == 0 {
		j.tlsConfig.MinVersion = tls.VersionTLS13
	}

	j.logger.Debug().Msgf("ServeQUIC: listen %s - %s", network, j.addr)
	quicConf := &quic.Config{TLSConfig: j.tlsConfig}
	endp, err := quic.Listen(network, j.addr, quicConf)
	if err != nil {
		return nil, fmt.Errorf("ServeQUIC Listen failed: %w", err)
	}

	acceptDone := make(chan error)

	j.URL = &url.URL{Scheme: j.Parameters.Protocol, Host: endp.LocalAddr().String()}
	j.Logger().Warn().Msgf("L4 QUIC server %s listening %s (URL = %s)", j.Parameters.Protocol, endp.LocalAddr(), j.URL)

	// signal we're ready  to accept connections
	wg.Done()
	done = true

	//spawn the accept loop
	//result := make(chan metric.Metric)
	var running atomic.Bool
	running.Store(true)
	go func() {
		for {
			conn, err := endp.Accept(ctx)
			fmt.Println("ACCEPT QUIC", err)
			if err != nil {
				j.Logger().Debug().Err(err).Msgf("accept failed with:")
				if running.Load() {
					acceptDone <- err
				} else {
					acceptDone <- nil
				}
				break
			}
			j.logger.Info().Msgf("L4 QUIC-GO server : incoming new connection from %s", conn.RemoteAddr())
			j.TotalRequests.Add(1)
			// spawn handling of streams for this connection
			go handleQUICStream(ctx, j, endp, conn)
		}
	}()
	err = nil
	j.Logger().Debug().Msgf("entering wait loop")
	select {
	case err = <-acceptDone:
		fmt.Println("DONE")
	case <-ctx.Done():
		running.Store(false)
		fmt.Println("CONTEXT")
		// todo , we silently ignore context error (cancel/timeout)
		err = nil
	}
	j.Logger().Debug().Err(err).Msgf("L4 server ending")

	//todo: handle pending connections
	_ = endp.Close(context.Background())
	// wait for sub go routine (ls.Accept should unblock with an "use of closed network connection") to end
	<-acceptDone
	j.Logger().Debug().Err(err).Msgf("L4 server ended")
	return &metric.Metric{}, err

}

func handleQUICStream(ctx context.Context, j *L4Server, endp *quic.Endpoint, conn *quic.Conn) {
	//for {
	str, err := conn.AcceptStream(ctx)
	fmt.Println("ACCEPT STREAM", err)
	if err != nil {
		j.Logger().Info().Err(err).Msgf("L4 QUIC server quic.AcceptStream failed")
		return
	}
	go j.handleConnection(quicStreamAsConn{str, endp, conn, j.id})
	//}
}
