package nspeed

import (
	"net"

	"github.com/quic-go/quic-go"
	"github.com/rs/zerolog"
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

//	type Conn interface {
//		Read(b []byte) (n int, err error)
//		Write(b []byte) (n int, err error)
//		Close() error
//		LocalAddr() net.Addr
//		RemoteAddr() net.Addr
//		SetDeadline(t time.Time) error
//		SetReadDeadline(t time.Time) error
//		SetWriteDeadline(t time.Time) error
//	}
//
// implements these above to be a net.Conn
