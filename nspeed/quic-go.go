package nspeed

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/quic-go/quic-go"
	"github.com/rs/zerolog"
	"nspeed.app/metric"
	"nspeed.app/utils"
)

// QUIC stuff

func quicInit(j *JobClient) (*quic.Config, *quic.Transport) {
	quicConf := &quic.Config{}
	if zerolog.GlobalLevel() <= zerolog.TraceLevel {
		quicConf.Tracer = utils.GenerateQLogTracer(j.Logger())
	} else {
		quicConf.Tracer = NewQuicTracer(j.wm)
	}

	// with use http3.RoundTripper but it doesnt expose its qTransport member field so we must use our own
	var qTransport *quic.Transport
	defer func() {
		if qTransport != nil {
			j.logger.Debug().Msg("closing local quic transport")
			qTransport.Close()
			qTransport = nil
		}
	}()

	// since we use Dialcontext we need to set a proper local *net.UDPAddr for LocalAddr
	if j.localAddr != nil {
		j.dialer.LocalAddr = &net.UDPAddr{
			IP:   j.localAddr.IP,
			Zone: j.localAddr.Zone,
		}
	}
	return quicConf, qTransport
}

// ConnStream make a quic Steam a net.Conn interface
type ConnStream struct {
	quic.Stream
	c quic.Connection
}

func (str ConnStream) LocalAddr() net.Addr {
	return str.c.LocalAddr()
}

func (str ConnStream) RemoteAddr() net.Addr {
	return str.c.RemoteAddr()
}

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

	udpConn, err := listenUDP(j.addr, j.Parameters.IPVersion)
	if err != nil {
		return nil, fmt.Errorf("ServeQUIC listenUDP error: %w", err)
	}
	j.logger.Debug().Msgf("ServerQUIC: listen %s - %s", network, j.addr)
	quicConf := &quic.Config{}
	quicTransport := quic.Transport{
		Conn: udpConn,
	}

	ln, err := quicTransport.Listen(j.tlsConfig, quicConf)
	if err != nil {
		return nil, fmt.Errorf("ServeQUIC listen failed: %w", err)
	}

	acceptDone := make(chan error)

	j.URL = &url.URL{Scheme: j.Parameters.Protocol, Host: ln.Addr().String()}
	j.Logger().Warn().Msgf("L4 QUIC-GO server %s listening %s (URL = %s)", j.Parameters.Protocol, ln.Addr(), j.URL)

	// signal we're ready  to accept connections
	wg.Done()
	done = true

	//spawn the accept loop
	//result := make(chan metric.Metric)
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
			go handleQUICStreams(ctx, j, conn)
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
	j.Logger().Debug().Err(err).Msgf("L4 server ending")

	//todo: handle pending connections
	_ = ln.Close()
	// wait for sub go routine (ls.Accept should unblock with an "use of closed network connection") to end
	<-acceptDone
	j.Logger().Debug().Err(err).Msgf("L4 server ended")
	return &metric.Metric{}, err

}

func handleQUICStreams(ctx context.Context, j *L4Server, conn quic.Connection) {
	for {
		str, err := conn.AcceptStream(ctx)
		if err != nil {
			j.Logger().Info().Err(err).Msgf("L4 QUIC server conn.AcceptStream failed or ended")
			return
		}
		go j.handleConnection(ConnStream{str, conn})
	}
}
