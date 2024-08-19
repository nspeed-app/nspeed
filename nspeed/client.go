package nspeed

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"reflect"
	"slices"
	"syscall"
	"time"

	"github.com/quic-go/quic-go"
	"nspeed.app/metric"
	"nspeed.app/utils"
)

func (j *JobClient) L4Init() error {

	if !slices.Contains(L4Protocols[:], j.Protocol) {
		return fmt.Errorf("protocol %s not supported", j.Protocol)
	}
	return nil
}

func (j *JobClient) L4Run(ctx context.Context, opt Options) (*metric.Metric, error) {

	if !slices.Contains(L4Protocols[:], j.Protocol) {
		return nil, fmt.Errorf("unknown protocol: %s", j.Protocol)
	}

	dialerContext := j.dialContext
	network := j.Protocol

	switch j.Protocol {
	case "tcp":
		network = utils.AddIPVersionToNetwork(network, j.IPVersion)
		if j.tlsConfig == nil {
			j.Logger().Info().Msgf("L4 client dialing %s %s", network, j.targetURL.Host)
		} else {
			tlsDialer := tls.Dialer{
				NetDialer: j.dialer,
				Config:    j.tlsConfig,
			}
			dialerContext = tlsDialer.DialContext
			j.Logger().Info().Msgf("L4 client dialing %s %s with TLS", network, j.targetURL.Host)
		}
	case "quic":
		return nil, fmt.Errorf("%s NYI", j.Protocol)

	case "quic-go":
		quicConfig, quicTransport := quicInit(j)
		if j.tlsConfig == nil {
			j.tlsConfig = &tls.Config{}
		}
		dialerContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			addr, err := net.ResolveUDPAddr("udp", address)
			if err != nil {
				return nil, fmt.Errorf("error %v resolving '%s' (%s)", err, address, network)
			}

			// todo: move this in a function, it's also used in http client
			if quicTransport == nil {
				// check for Zone on IPv6 link-local for instance, some OS might need the Zone too
				udpConn, err := net.ListenUDP("udp", nil)
				if err != nil {
					return nil, fmt.Errorf("error for local port: %v", err)
				}
				quicTransport = &quic.Transport{Conn: udpConn}
			}
			conn, err := quicTransport.Dial(ctx, addr, j.tlsConfig, quicConfig)
			if err != nil {
				return nil, fmt.Errorf("error quic dialing (%w)", err)
			}
			str, err := conn.OpenStream()
			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				return nil, fmt.Errorf("error quic dialing (%w)", err)
			}
			return ConnStream{str, conn}, nil
		}

	default:
		return nil, fmt.Errorf("unknown protocol %s", j.Protocol)
	}

	conn, err := dialerContext(ctx, network, j.targetURL.Host)

	if err != nil {
		return nil, fmt.Errorf("dial error: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(j.Timeout)); err != nil {
		return nil, fmt.Errorf("SetDeadline error: %w", err)
	}

	if j.ContentSize == 0 {
		return nil, fmt.Errorf("no content to send")
	}

	// setup the metric
	j.wm = &metric.Metric{
		MetricsData: metric.MetricsData{
			Done:    false,
			Expired: false,
			Target:  j.GetTarget(),
			ID:      j.ID(),
			GroupID: j.GroupID(),
			Command: j.GoString(),
		},
		Tick: opt.Tick,
	}
	// fill in metadata
	j.wm.Remote, err = netip.ParseAddrPort(conn.RemoteAddr().String())
	if err != nil {
		j.Logger().Debug().Err(err).Msgf("error parsing remote address: %s", conn.RemoteAddr().String())
	}
	j.wm.SourceAddr, err = netip.ParseAddrPort(conn.LocalAddr().String())
	if err != nil {
		j.Logger().Debug().Err(err).Msgf("error parsing remote address: %s", conn.RemoteAddr().String())
	}
	j.wm.Interface, j.wm.Gateway, j.wm.RouteSourceAddr = utils.GetRoute(conn.RemoteAddr().String())
	j.wm.SetTLSInfo(conn)
	// set the logger for this metric
	j.wm.SetLogger(j.Logger())

	// init metric data
	j.wm.WriteStepSize = min(utils.MaxChunkSize, 1*1024*1024)
	j.wm.ReadStepSize = min(utils.MaxChunkSize, 1*1024*1024)
	chunk := utils.GetChunk(j.wm.WriteStepSize)

	cmd := ""
	total := int64(0)
	iostep := j.wm.WriteStepSize
	var iocmd func(n int64) (int64, error)
	var end func()

	msg := NewL4Message(j.Command, j.ContentSize)
	// POST is same a PUT server side
	if j.Command == "POST" {
		msg.Command = "PUT"
	}
	err = WriteL4Message(conn, &msg)
	if err != nil {
		j.wm.Error = err
		return nil, fmt.Errorf("write message failed: %w", err)
	}

	// transmit
	if msg.Command == "PUT" {
		cmd = "sending"
		j.wm.ContentWriteSize = j.ContentSize
		j.wm.TotalWrite = 0
		iocmd = func(n int64) (int64, error) {
			nn, err := j.wm.ReadN(n)
			if err != nil { // ReadN only returns io.EOF
				j.Logger().Trace().Msgf("L4 client ReadN ending (err = %+v)", err)
				j.wm.Done = true
				return 0, err
			}
			nw, err := conn.Write(chunk[:min(j.wm.WriteStepSize, nn)])
			return int64(nw), err
		}
		end = func() {
			j.wm.TotalWrite = total
			j.Logger().Info().Msgf("L4 client sent %d bytes to %s", total, j.targetURL)
		}
	}
	// receive
	if msg.Command == "GET" {
		cmd = "receiving"
		j.wm.ContentReadSize = j.ContentSize
		j.wm.TotalRead = 0
		iocmd = func(n int64) (int64, error) {
			nw, err := conn.Read(chunk[:j.wm.ReadStepSize])

			nn := j.wm.WriteN(int64(nw))

			if j.wm.TotalRead >= j.wm.ContentReadSize {
				return nn, io.EOF
			}
			return int64(nw), err
		}
		end = func() {
			j.wm.TotalRead = total
			j.Logger().Info().Msgf("L4 client got %d bytes from %s", total, j.targetURL)
		}
	}
	// safe guard, should'nt happend
	if iocmd == nil {
		return j.wm, fmt.Errorf("no command")
	}
	j.wm.InitNew()

	j.Logger().Info().Msgf("L4 client %s %d bytes remote %s", cmd, j.wm.ContentWriteSize, j.targetURL)
	// spawn the work, use 'done' to signal
	done := make(chan any)

	j.wm.StartTicker(done)

	go func() {
		for {
			nw, err := iocmd(iostep)

			total += int64(nw)
			j.Logger().Trace().Msgf("L4 client %s %d bytes remote = %s (err=%s)", cmd, nw, j.targetURL, reflect.TypeOf(err))
			// possible error here:
			//  timeout because of Timeout or network error (both = a net.OpError error type)
			// either way we mark as 'cancelled' if timeout and return no error ?
			if err == io.EOF {
				j.wm.Error = nil
				break
			}
			if err != nil {
				j.wm.Error = fmt.Errorf("conn io error: %w", err)
				var opError *net.OpError
				if errors.As(err, &opError) {
					if opError.Timeout() {
						j.wm.Cancelled = true
						j.wm.Error = nil
					}
				}
				//quic-go generates os.ErrDeadlineExceeded
				if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, os.ErrDeadlineExceeded) {
					j.wm.Cancelled = true
					j.wm.Error = nil
				}
				break
			}
		}
		close(done)
	}()

	// wait for completion or context
	select {
	case <-ctx.Done():
		j.wm.Cancelled = true
	case <-done:
	}
	// adjust totalwrite & elaspsedTime (which iare updated every call to j.wm.ReadN(...)) but can be less if conn.Write get time'd out
	j.wm.ElapsedTime = time.Since(j.wm.TransfertStartTime)
	if total > 0 {
		j.wm.Valid = true
	}
	end()
	if j.wm.Error != nil {
		j.Logger().Info().Err(j.wm.Error).Msgf("L4 client error")
	}
	j.wm.Command = j.PrintString(false, false)
	return j.wm, j.wm.Error
}
