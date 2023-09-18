package nspeed

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"reflect"
	"sync"
	"syscall"
	"time"

	"nspeed.app/metric"
	"nspeed.app/utils"
)

func (j *JobClient) L4Init() error {

	if j.Protocol != "tcp" {
		return fmt.Errorf("protocol %s not yet supported", j.Protocol)
	}
	return nil
}

func (j *JobClient) L4Run(ctx context.Context, wg *sync.WaitGroup) (*metric.Metric, error) {
	defer wg.Done()

	if j.Protocol != "tcp" {
		return nil, fmt.Errorf("protocol %s not yet supported", j.Protocol)
	}

	network := j.Protocol
	network = utils.AddIPVersionToNetwork(network, j.IPVersion, GlobalOptions.IPVersion)

	dialerContext := j.dialContext
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
		Tick: GlobalOptions.Tick,
	}
	// fill in metadata
	j.wm.Interface, j.wm.Gateway, j.wm.SourceAddr = utils.GetRoute(conn.RemoteAddr().String())
	if tlsConn, ok := conn.(*tls.Conn); ok {
		cs := tlsConn.ConnectionState()
		j.wm.Protocol = "TCP"
		j.wm.Cipher = TLSCipherSuite(&cs)
		j.wm.TLSVersion = utils.TLSVersionFriendlyName(cs.Version)
	}
	// set the logger for this metric
	j.wm.SetLogger(j.Logger())

	// init metric data
	j.wm.WriteStepSize = min(utils.MaxChunkSize, 1*1024*1024)
	j.wm.ReadStepSize = min(utils.MaxChunkSize, 1*1024*1024)
	chunk := utils.GetChunk(j.wm.WriteStepSize)

	fmt.Println("COMMAND = ", j.Command, j.ContentSize)

	cmd := "receiving"
	total := int64(0)
	iocmd := j.wm.ReadN
	iostep := j.wm.WriteStepSize
	var end func()
	if j.Command == "PUT" || j.Command == "POST" {
		cmd = "sending"
		j.wm.ContentWriteSize = j.ContentSize
		j.wm.TotalWrite = 0
		end = func() {
			j.wm.TotalWrite = total
			j.Logger().Info().Msgf("L4 client sent %d bytes to %s", total, j.targetURL)
		}
	}
	if j.Command == "GET" {
		j.wm.ContentReadSize = j.ContentSize
		j.wm.TotalRead = 0
		iocmd = func(n int64) (int64, error) {
			return j.wm.WriteN(n), nil
		}
		end = func() {
			j.wm.TotalRead = total
			j.Logger().Info().Msgf("L4 client got %d bytes from %s", total, j.targetURL)
		}
	}
	j.wm.InitNew()

	j.Logger().Info().Msgf("L4 client %s %d bytes to %s", cmd, j.wm.ContentWriteSize, j.targetURL)
	// spawn the work, use 'done' to signal
	done := make(chan bool)
	go func() {
		for {
			n, err := iocmd(iostep)
			if err != nil { // ReadN only returns io.EOF
				j.Logger().Trace().Msgf("L4 client ReadN ending (err = %+v)", err)
				j.wm.Done = true
				break
			}
			nw, err := conn.Write(chunk[:min(j.wm.WriteStepSize, n)])

			total += int64(nw)
			j.Logger().Trace().Msgf("L4 client %s %d bytes to %s (err=%s)", cmd, nw, j.targetURL, reflect.TypeOf(err))
			// possible error here:
			//  timeout because of Timeout or network error (both = a net.OpError error type)
			// either way we mark as 'cancelled' if timeout and return no error ?
			if err != nil {
				j.wm.Error = fmt.Errorf("conn io error: %w", err)
				var opError *net.OpError
				if errors.As(err, &opError) {
					if opError.Timeout() {
						j.wm.Cancelled = true
						j.wm.Error = nil
					}
				}
				if errors.Is(err, syscall.ECONNRESET) {
					j.wm.Cancelled = true
					j.wm.Error = nil
				}
				break
			}
		}
		done <- true
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
	return j.wm, j.wm.Error
}
