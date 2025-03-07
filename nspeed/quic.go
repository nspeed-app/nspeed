package nspeed

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"

	"golang.org/x/net/quic"
	"nspeed.app/metrics"
	"nspeed.app/utils"
)

// func quicClientInit(j *JobClient) (*quic.Config, *quic.Endpoint) {
// 	quicConf := &quic.Config{}
// 	endp := &quic.Endpoint{}

// 	// since we use Dialcontext we need to set a proper local *net.UDPAddr for LocalAddr
// 	if j.localAddr != nil {
// 		j.dialer.LocalAddr = &net.UDPAddr{
// 			IP:   j.localAddr.IP,
// 			Zone: j.localAddr.Zone,
// 		}
// 	}
// 	return quicConf, endp
// }

// SERVER

func (j *L4Server) ServeQUIC(ctx context.Context, network string, wg *sync.WaitGroup) (*metrics.Metric, error) {
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
	//result := make(chan metrics.Metric)
	var running atomic.Bool
	running.Store(true)
	go func() {
		for {
			conn, err := endp.Accept(ctx)
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
			go handleQUICStream(ctx, j, conn)
			conn = nil
		}
	}()
	err = nil
	j.Logger().Debug().Msgf("entering wait loop")
	accepted := false
	select {
	case err = <-acceptDone:
		accepted = true
	case <-ctx.Done():
		running.Store(false)
		// todo , we silently ignore context error (cancel/timeout)
		err = nil
	}
	j.Logger().Debug().Err(err).Msgf("L4 server ending")

	//todo: handle pending connections
	_ = endp.Close(ctx)
	// wait for sub go routine (Accept should unblock with an "use of closed network connection") to end
	if !accepted {
		<-acceptDone
	}
	j.Logger().Debug().Err(err).Msgf("L4 server ended")
	return &metrics.Metric{}, err

}

func handleQUICStream(ctx context.Context, j *L4Server, conn *quic.Conn) {
	for {
		str, err := conn.AcceptStream(ctx)
		if err != nil {
			j.Logger().Info().Err(err).Msgf("L4 QUIC server quic.AcceptStream failed")
			return
		}
		c := utils.NewQuicStreamAsConn(str, conn)
		go func() {
			j.handleConnection(c)
			conn.Abort(nil)
			err = conn.Wait(context.Background())
			//log.Debug().Msgf("L4 quic conn.Wait: %s", err)
		}()
	}
}
