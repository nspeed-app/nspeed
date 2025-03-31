package nspeed

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"nspeed/metrics"

	"github.com/quic-go/quic-go"
	quicgo "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/quic-go/logging"
	"github.com/quic-go/quic-go/qlog"
	"github.com/rs/zerolog"
	"nspeed.app/network"
)

// quicgoStreamAsConn make a quicgo Stream a net.Conn interface
type quicgoStreamAsConn struct {
	quicgo.Stream
	c quicgo.Connection
}

func (str quicgoStreamAsConn) LocalAddr() net.Addr {
	return str.c.LocalAddr()
}

func (str quicgoStreamAsConn) RemoteAddr() net.Addr {
	return str.c.RemoteAddr()
}

func (j *L4Server) ServeQUICGO(ctx context.Context, network string, wg *sync.WaitGroup) (*metrics.Metric, error) {
	// this avoid calling wg.Done() twice
	done := false
	defer func() {
		if !done {
			wg.Done()
		}
	}()

	if j.tlsConfig == nil {
		return nil, fmt.Errorf("ServeQUICGO: no TLS config")
	}

	udpConn, err := listenUDP(j.addr, j.Parameters.IPVersion)
	if err != nil {
		return nil, fmt.Errorf("ServeQUICGO listenUDP error: %w", err)
	}
	j.logger.Debug().Msgf("ServerQUIC: listen %s - %s", network, j.addr)
	quicConf := &quicgo.Config{}
	quicTransport := quicgo.Transport{
		Conn: udpConn,
	}

	ln, err := quicTransport.Listen(j.tlsConfig, quicConf)
	if err != nil {
		return nil, fmt.Errorf("ServeQUICGO listen failed: %w", err)
	}

	acceptDone := make(chan error)

	j.URL = &url.URL{Scheme: strings.ToLower(j.Parameters.Protocol), Host: ln.Addr().String()}
	j.Logger().Warn().Msgf("L4 %s server listening %s (URL = %s)", j.Parameters.Protocol, ln.Addr(), j.URL)

	// signal we're ready  to accept connections
	wg.Done()
	done = true

	//spawn the accept loop
	//result := make(chan metrics.Metric)
	var running atomic.Bool
	running.Store(true)
	go func() {
		for {
			conn, err := ln.Accept(ctx)
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
			go handleQUICGOStreams(ctx, j, conn)
		}
	}()
	err = nil
	j.Logger().Debug().Msgf("entering wait loop")
	select {
	case err = <-acceptDone:
	case <-ctx.Done():
		running.Store(false)
		// todo , we silently ignore context error (cancel/timeout)
		err = nil
	}
	j.Logger().Debug().Err(err).Msgf("L4 QUIC-GO server ending")

	//todo: handle pending connections
	_ = ln.Close()
	// wait for sub go routine (ls.Accept should unblock with an "use of closed network connection") to end
	<-acceptDone
	j.Logger().Debug().Err(err).Msgf("L4 QUIC-GO server ended")
	return &metrics.Metric{}, err

}

func handleQUICGOStreams(ctx context.Context, j *L4Server, conn quicgo.Connection) {
	for {
		str, err := conn.AcceptStream(ctx)
		if err != nil {
			j.Logger().Info().Err(err).Msgf("L4 QUIC-GO server AcceptStream failed or ended")
			return
		}
		go j.handleConnection(quicgoStreamAsConn{str, conn})
	}
}

func GenerateQLogTracer(log *zerolog.Logger) func(ctx context.Context, p logging.Perspective, connID quic.ConnectionID) *logging.ConnectionTracer {
	QLogTracer := func(ctx context.Context, p logging.Perspective, connID quic.ConnectionID) *logging.ConnectionTracer {
		perspectiveNames := [...]string{"server", "client"}
		perspectiveName := func(p logging.Perspective) string {
			if p != 1 && p != 2 {
				return "unknown"
			}
			return perspectiveNames[p-1]
		}
		filename := fmt.Sprintf("%s_%x.qlog", perspectiveName(p), connID)
		f, err := os.Create(filename)
		if err != nil {
			log.Fatal().Err(err).Send()
		}
		log.Trace().Msgf("Creating qlog file %s.\n", filename)
		return qlog.NewConnectionTracer(network.NewBufferedWriteCloser(bufio.NewWriter(f), f), p, connID)
	}
	return QLogTracer
}

// client

// todo: remove dep from JobClient using a new interface or more arguments.

func quicgoInit(j *JobClient) *quicgo.Config {
	quicConf := &quicgo.Config{}
	if zerolog.GlobalLevel() <= zerolog.TraceLevel {
		quicConf.Tracer = GenerateQLogTracer(j.Logger())
	} else {
		quicConf.Tracer = NewQuicTracer(j.wm)
	}

	// since we use Dialcontext we need to set a proper local *net.UDPAddr for LocalAddr
	if j.localAddr != nil {
		j.dialer.LocalAddr = &net.UDPAddr{
			IP:   j.localAddr.IP,
			Zone: j.localAddr.Zone,
		}
	}
	return quicConf
}

// todo: remove dep from JobClient using a new interface or more arguments.
func NewQUICGOHTTP3Transport(netTransport *http.Transport, j *JobClient) (http.RoundTripper, *io.Closer) {
	quicConf := quicgoInit(j)
	var qTransport *quicgo.Transport
	var closer io.Closer
	// the roundtrip
	rtTransport := &http3.Transport{
		DisableCompression: true,
		TLSClientConfig:    netTransport.TLSClientConfig,
		QUICConfig:         quicConf,
		Dial: func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quicgo.Config) (quicgo.EarlyConnection, error) {
			j.Logger().Debug().Msgf("HTTP3 pre dialing %s ", addr)
			udpAddr, err := netTransport.DialContext(ctx, "udp", addr)
			if err != nil {
				return nil, fmt.Errorf("htt3 Dial failed: %w", err)
			}
			if qTransport == nil {
				// check for Zone on IPv6 link-local for instance, some OS might need the Zone too
				udpConn, err := net.ListenUDP("udp", nil)
				if err != nil {
					return nil, err
				}
				fmt.Println("NEW QUIC UDP ENDPOINT", udpConn.LocalAddr())
				qTransport = &quicgo.Transport{Conn: udpConn}
				closer = udpConn
			}
			// use same dialer as other http version

			j.Logger().Debug().Msgf("HTTP3 dialing %s -> %s", addr, udpAddr.RemoteAddr().String())
			return qTransport.DialEarly(ctx, udpAddr.RemoteAddr(), tlsCfg, cfg)
		},
	}
	return rtTransport, &closer
}
