package nspeed

import (
	"context"
	"crypto/tls"
	"fmt"
	"math"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"

	"nspeed.app/nspeed/humanize"
	"nspeed.app/nspeed/metrics"

	"golang.org/x/net/quic"

	"nspeed.app/nspeed/network"
)

// UDP socket buffer size for x/net QUIC endpoints, same as quic-go. With the OS
// default (~208 KiB on Linux) bursts from windows above 1 MiB overflow the socket
// buffer, and the resulting losses make x/net's Reno congestion control slower.
// Linux caps it to net.core.rmem_max / wmem_max.
const quicSocketBufferSize = 7 << 20

// quicWindowFlagHelp documents the -quic-window flag of the server and client commands.
const quicWindowFlagHelp = "x/net QUIC (-P quic) flow-control window and send buffer size, in bytes or units of bytes (0 = library default of 1 MiB)"

// parseQUICWindow parses the -quic-window flag value.
func parseQUICWindow(s string) (int64, error) {
	v, err := humanize.ParseByteUnits(s)
	if err != nil {
		return 0, fmt.Errorf("invalid -quic-window %q: %w", s, err)
	}
	if v > math.MaxInt64 {
		return 0, fmt.Errorf("invalid -quic-window %q: too large", s)
	}
	return int64(v), nil
}

// newQUICConfig returns the x/net/quic configuration used by both client and server.
//
// window > 0 replaces the library's fixed 1 MiB stream/connection flow-control windows
// and stream send buffer. They cap a stream at about window bytes per RTT
// (1 MiB: ~420 Mbps at 20 ms RTT); the send buffer holds data until it is acknowledged,
// so it limits bytes in flight too. Larger values cost CPU: x/net walks the buffered
// send data (a list of 4 KiB chunks) for every packet, so they are slower on loopback.
// nspeed uses one stream per connection, so the connection window equals the stream window.
func newQUICConfig(tlsConfig *tls.Config, window int64) *quic.Config {
	return &quic.Config{
		TLSConfig:                tlsConfig,
		MaxStreamReadBufferSize:  window,
		MaxStreamWriteBufferSize: window,
		MaxConnReadBufferSize:    window,
	}
}

// listenQUIC is quic.Listen with the UDP socket buffers enlarged to quicSocketBufferSize.
// conf may be nil (the endpoint then only dials).
func listenQUIC(netw, address string, conf *quic.Config, logger *zerolog.Logger) (*quic.Endpoint, error) {
	addr, err := net.ResolveUDPAddr(netw, address)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP(netw, addr)
	if err != nil {
		return nil, err
	}
	if err := conn.SetReadBuffer(quicSocketBufferSize); err != nil {
		logger.Debug().Err(err).Msg("QUIC: failed to set UDP receive buffer size")
	}
	if err := conn.SetWriteBuffer(quicSocketBufferSize); err != nil {
		logger.Debug().Err(err).Msg("QUIC: failed to set UDP send buffer size")
	}
	endp, err := quic.NewEndpoint(conn, conf)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return endp, nil
}

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
	quicConf := newQUICConfig(j.tlsConfig, j.Parameters.QUICWindow)
	endp, err := listenQUIC(network, j.addr, quicConf, j.Logger())
	if err != nil {
		return nil, fmt.Errorf("ServeQUIC Listen failed: %w", err)
	}

	acceptDone := make(chan error)

	j.URL = &url.URL{Scheme: strings.ToLower(j.Parameters.Protocol), Host: endp.LocalAddr().String()}
	j.Logger().Warn().Msgf("L4 %s server listening %s (URL = %s)", j.Parameters.Protocol, endp.LocalAddr(), j.URL)

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
	j.Logger().Debug().Err(err).Msgf("L4 QUIC server ending")

	//todo: handle pending connections
	_ = endp.Close(ctx)
	// wait for sub go routine (Accept should unblock with an "use of closed network connection") to end
	if !accepted {
		<-acceptDone
	}
	j.Logger().Debug().Err(err).Msgf("L4 QUIC server ended")
	return &metrics.Metric{}, err

}

func handleQUICStream(ctx context.Context, j *L4Server, conn *quic.Conn) {
	for {
		str, err := conn.AcceptStream(ctx)
		if err != nil {
			j.Logger().Info().Err(err).Msgf("L4 QUIC server quic.AcceptStream failed")
			return
		}
		c := network.NewQuicStreamAsConn(str, conn)
		go func() {
			j.handleConnection(c)
			conn.Abort(nil)
			err = conn.Wait(context.Background())
			//log.Debug().Msgf("L4 quic conn.Wait: %s", err)
		}()
	}
}
