package nspeed

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"nspeed.app/nspeed/metrics"

	quicgo "github.com/quic-go/quic-go"
	"github.com/rs/zerolog"
	"golang.org/x/net/quic"
	"nspeed.app/nspeed/crypto"
	"nspeed.app/nspeed/generics"
	"nspeed.app/nspeed/humanize"
	"nspeed.app/nspeed/iobuffer"
	"nspeed.app/nspeed/network"
	"nspeed.app/nspeed/pacing"
)

// supported L4Protocols
var L4Protocols = [...]string{"TCP", "QUIC", "QUIC-GO"}

// a client command, get or post
// implements Job interface
type JobClient struct {
	JobData
	Command          string
	Protocol         string // "http" or one of the L4Protocols
	Instance         int    //=0 single, >0 index of instance
	Repeat           uint   // 0 or 1 is 1
	IPVersion        network.IPVersion
	Host             string
	Target           string
	Timeout          time.Duration
	DialTimeout      time.Duration
	UserAgent        string
	ContentType      string
	ContentSize      int64
	SkipTLSCheck     bool
	UseTLSSelfConfig bool
	Delay            time.Duration
	Schedule         *pacing.Schedule
	HTTPVersion      int
	H2C              bool
	MTU              uint16 // bigger one day ?
	// private
	localAddr   *net.IPAddr
	dialer      *net.Dialer
	dialContext func(ctx context.Context, network string, address string) (net.Conn, error)
	tlsConfig   *tls.Config
	client      *http.Client
	req         *http.Request
	tr          roundTripTrace
	numRedirect int
	finalTarget string
	targetURL   *url.URL
	wm          *metrics.Metric
	closer      *io.Closer
}

func (j *JobClient) GoString() string {
	if zerolog.GlobalLevel() > zerolog.InfoLevel {
		return fmt.Sprintf("%s %s %s", j.Protocol, j.Command, j.Target)
	}
	return j.CommandString()
}

func (j *JobClient) CommandString() string {
	s := fmt.Sprintf("%s client {Method: %s, URL: %s, IPversion: %d, Address: %s, timeout: %s, delay: %s, h2c: %t, http:%s}",
		j.Protocol,
		j.Command,
		j.Target,
		j.IPVersion,
		j.Host,
		j.Timeout.String(),
		j.Delay.String(),
		j.H2C,
		network.HTTPVersionStringer(j.HTTPVersion),
	)
	if j.ContentSize != 0 {
		s += fmt.Sprintf(",size: %d (%sB)", j.ContentSize, humanize.ByteCountDecimal(j.ContentSize))
	}
	s += "}"
	return s
}

func (j *JobClient) GetTarget() string {
	return j.Target
}

func (j *JobClient) GetConn() *httptrace.GotConnInfo {
	if j.wm != nil {
		return &j.tr.ConnInfo
	}
	return nil
}

func (j *JobClient) GetDestination() string {
	ci := j.GetConn()
	if ci == nil {
		return ""
	}
	if ci.Conn == nil {
		return ""
	}
	return ci.Conn.RemoteAddr().String()
}

// Pretty print command and some infos
// todo: fix me
func (j *JobClient) PrintString(verbose bool, group bool) string {

	formatOut := func(r string) string {
		if j.wm.AvgLatency == 0 {
			//todo: move this elsewhere
			j.wm.AvgLatency = j.tr.connectDone.Sub(j.tr.connectStart)
		}
		latency := j.wm.AvgLatency.Milliseconds()
		latencyMicroRem := j.wm.AvgLatency.Microseconds() % 1000
		ipv := network.GetIPVersion(j.wm.Remote.Addr())
		details := fmt.Sprintf("%s - %d/%d(%s - %d.%d ms - %s - %s)", r, j.wm.Loop, j.wm.LoopMax, ipv, latency, latencyMicroRem, j.wm.Protocol, j.wm.TLSVersion)
		if j.numRedirect > 0 && verbose {
			return fmt.Sprintf("%s from %s", details, j.Target)
		}
		return details
	}

	groupFormat := ""
	if group {
		groupFormat = "-n " + strconv.Itoa(j.Instance) + " "
	}

	switch j.Command {
	case "GET":
		t := "get " + groupFormat + "%s" + j.finalTarget
		r := fmt.Sprintf(t, network.HTTPVersionFlag(j.HTTPVersion, j.H2C))
		return formatOut(r)

	case "POST", "PUT":
		t := strings.ToLower(j.Command) + " " + groupFormat + "%s" + j.finalTarget + " " + humanize.ByteCountDecimal(j.ContentSize) + "B" //todo
		r := fmt.Sprintf(t, network.HTTPVersionFlag(j.HTTPVersion, j.H2C))
		return formatOut(r)

	default:
		return "unknown command"
	}
}

func (j *JobClient) IsInfinite() bool { return false }

// init a HTTP Job
func (j *JobClient) Init(opt Options) error {

	// timeout (if none is specified use global default)
	j.Timeout = generics.OrDefault(j.Timeout, opt.DefaultClientTimeout)

	// todo move this to run ? and get ride of .wm?
	j.wm = &metrics.Metric{
		MetricsData: metrics.MetricsData{
			Done:    false,
			Expired: false,
			Target:  j.GetTarget(),
			ID:      j.ID(),
			GroupID: j.GroupID(),
			Command: j.CommandString(),
		},
		Instance: j.Instance,
		Tick:     opt.Tick,
	}
	j.wm.SetLogger(j.Logger())

	// prepend https scheme if no scheme
	if !strings.Contains(j.Target, "://") {
		j.Target = "https://" + j.Target
	}

	if !j.IPVersion.IsValid() {
		return errors.New("invalid ip version requested")
	}
	j.IPVersion = generics.OrDefault(j.IPVersion, opt.IPVersion)
	// source address & port
	var err error
	// j.Host can have an optionnal trailing ":port"
	localAddr, localPort, err := network.ParseAddressWithOptionnalPort(j.Host)
	if err != nil {
		return fmt.Errorf("InterfaceAddress ParseOtionnalPort error: %w", err)
	}
	j.localAddr, err = network.InterfaceAddress(localAddr, j.IPVersion)
	if err != nil {
		return fmt.Errorf("InterfaceAddress error: %w", err)
	}
	// we cast a to *TCPAddr but change later for udp as needed because Dial performs a type switch
	// and we get a  "mismatched local address type" error
	localTCPAddr := func() net.Addr {
		if j.localAddr == nil {
			return nil
		}
		return &net.TCPAddr{
			IP:   j.localAddr.IP,
			Zone: j.localAddr.Zone,
			Port: localPort,
		}
	}

	j.targetURL, err = ResolveSchemes(j.jobList.Jobs, j.Target)
	if err != nil {
		return fmt.Errorf("ResolveSchemes error: %w", err)
	}
	j.finalTarget = j.targetURL.String()
	j.Logger().Info().Msgf("real target is: %s", j.finalTarget)

	// TLS config
	if j.UseTLSSelfConfig {
		j.tlsConfig = crypto.GetTLSSelfConfig()
		j.Logger().Info().Msgf("using TLSSelfConfig")
		// for tls 1.3 we can't change cipher suites with current Go implementation: https://go.dev/blog/tls-cipher-suites
		// so something like this won't work:
		// netTransport.TLSClientConfig.CipherSuites = []uint16{tls.TLS_CHACHA20_POLY1305_SHA256}
		// netTransport.TLSClientConfig.MinVersion = tls.VersionTLS13
		// so we cannot (for now?) specify a cipher
	}
	if j.SkipTLSCheck {
		j.tlsConfig = &tls.Config{InsecureSkipVerify: true}
		j.Logger().Info().Msgf("skipping TLS Check")
	}

	if j.tlsConfig != nil {
		j.Logger().Debug().Msgf("TLSConfig: defined")
	}

	// Dialer
	j.dialer = &net.Dialer{
		Timeout:       j.DialTimeout,  //
		FallbackDelay: -1,             // don't use Happy Eyeballs
		LocalAddr:     localTCPAddr(), //localTCPAddr,  // "-a" for client
	}

	// this is our dialContext for transports/l4
	j.dialContext = func(ctx context.Context, netwrk string, address string) (net.Conn, error) {
		if netwrk == "udp" || netwrk == "tcp" {
			netwrk = network.AddIPVersionToNetwork(netwrk, j.IPVersion)
		}
		j.Logger().Debug().Msgf("DialContext %s %s %d (%s)", netwrk, address, j.IPVersion, j.dialer.LocalAddr)
		return j.dialer.DialContext(ctx, netwrk, address)
	}

	// check Repeat
	if j.Repeat == 0 {
		j.Logger().Debug().Msgf("Repeat set to 1")
		j.Repeat = 1
	}
	// branch to L4 or HTTP client
	switch j.targetURL.Scheme {
	case "http", "https":
		j.Protocol = "HTTP"
		return j.HTTPInit()
	case "tcp", "udp", "quic", "quic-go":
		j.Protocol = strings.ToUpper(j.targetURL.Scheme)
		return j.L4Init()
	default:
		return fmt.Errorf("invalid scheme: %s", j.targetURL.Scheme)
	}
}

func (j *JobClient) Close() error {
	if j.closer == nil {
		return nil
	}
	if *j.closer == nil {
		return nil
	}
	err := (*j.closer).Close()
	j.closer = nil
	return err
}

func (j *JobClient) Run(ctx context.Context, wg *sync.WaitGroup, opt Options) (*metrics.Metric, error) {
	defer func() {
		wg.Done()
		_ = j.Close()
	}()
	if j.Protocol == "HTTP" {
		return j.HTTPRun(ctx, opt)
	}
	return j.L4Run(ctx, opt)
}

func (j *JobClient) L4Init() error {

	if !slices.Contains(L4Protocols[:], j.Protocol) {
		return fmt.Errorf("protocol %s not supported", j.Protocol)
	}
	return nil
}

func (j *JobClient) L4Run(ctx context.Context, opt Options) (*metrics.Metric, error) {

	if !slices.Contains(L4Protocols[:], j.Protocol) {
		return nil, fmt.Errorf("unknown protocol: %s", j.Protocol)
	}

	dialerContext := j.dialContext
	netw := strings.ToLower(j.Protocol)

	switch j.Protocol {
	case "TCP":
		netw = network.AddIPVersionToNetwork(netw, j.IPVersion)
		if j.tlsConfig == nil {
			j.Logger().Info().Msgf("L4 client dialing %s %s", netw, j.targetURL.Host)
		} else {
			tlsDialer := tls.Dialer{
				NetDialer: j.dialer,
				Config:    j.tlsConfig,
			}
			dialerContext = tlsDialer.DialContext
			j.Logger().Info().Msgf("L4 client dialing %s %s with TLS", netw, j.targetURL.Host)
		}
	case "QUIC":
		if j.tlsConfig == nil {
			return nil, fmt.Errorf("quic client error: no tls config")
		}

		if j.tlsConfig.MinVersion == 0 {
			j.tlsConfig.MinVersion = tls.VersionTLS13
		}

		netw = network.AddIPVersionToNetwork("udp", j.IPVersion)

		quicConf := &quic.Config{TLSConfig: j.tlsConfig}

		localAddr := ":0"
		if j.localAddr != nil {
			j.dialer.LocalAddr = &net.UDPAddr{
				IP:   j.localAddr.IP,
				Zone: j.localAddr.Zone,
			}
			localAddr = j.dialer.LocalAddr.String()
		}

		endp, err := quic.Listen(netw, localAddr, nil)

		if err != nil {
			return nil, fmt.Errorf("L4 client quic listen error: %w", err)
		}

		dialerContext = func(ctx context.Context, netwrk, address string) (net.Conn, error) {

			j.Logger().Info().Msgf("L4 QUIC client dialing %s %s", netwrk, j.targetURL.Host)
			qconn, err := endp.Dial(ctx, netwrk, address, quicConf)
			if err != nil {
				return nil, fmt.Errorf("quic dial error: %w", err)
			}
			j.Logger().Info().Msgf("L4 QUIC client connected to %s %s", netwrk, j.targetURL.Host)
			stream, err := qconn.NewStream(ctx)
			if err != nil {
				return nil, fmt.Errorf("quic new stream error: %w", err)
			}
			c := network.NewQuicStreamAsConn(stream, qconn)
			return c, nil
		}

	case "QUIC-GO":
		quicConfig := quicgoInit(j)
		var quicTransport *quicgo.Transport
		if j.tlsConfig == nil {
			return nil, fmt.Errorf("quic-go client error: no tls config")
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
				quicTransport = &quicgo.Transport{Conn: udpConn}
				var closer io.Closer = udpConn
				j.closer = &closer
			}
			conn, err := quicTransport.Dial(ctx, addr, j.tlsConfig, quicConfig)
			if err != nil {
				return nil, fmt.Errorf("error quic dialing (%w)", err)
			}
			str, err := conn.OpenStream()
			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				return nil, fmt.Errorf("error quic dialing (%w)", err)
			}
			return quicgoStreamAsConn{str, conn}, nil
		}

	default:
		return nil, fmt.Errorf("unknown protocol %s", j.Protocol)
	}

	conn, err := dialerContext(ctx, netw, j.targetURL.Host)

	if err != nil {
		return nil, fmt.Errorf("dial error: %w", err)
	}
	// nolint: errcheck
	defer conn.Close()

	if j.Timeout != 0 {
		if err := conn.SetDeadline(time.Now().Add(j.Timeout)); err != nil {
			return nil, fmt.Errorf("SetDeadline error: %w", err)
		}
	}

	if j.ContentSize == 0 {
		return nil, fmt.Errorf("no content to send")
	}

	// setup the metric
	j.wm = &metrics.Metric{
		MetricsData: metrics.MetricsData{
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
	j.wm.Interface, j.wm.Gateway, j.wm.RouteSourceAddr, _ = network.GetRouteFromAddrPort(conn.RemoteAddr().String())
	j.wm.SetTLSInfo(conn)
	// set the logger for this metric
	j.wm.SetLogger(j.Logger())

	// init metric data
	j.wm.WriteStepSize = min(iobuffer.MaxBufferSize, 1*1024*1024)
	j.wm.ReadStepSize = min(iobuffer.MaxBufferSize, 1*1024*1024)
	chunk := iobuffer.GetChunk(j.wm.WriteStepSize)

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
		var writer io.Writer = conn
		if j.Schedule != nil {
			writer = pacing.NewPacedWriter(ctx, conn, j.Schedule)
		}

		iocmd = func(n int64) (int64, error) {
			nn, err := j.wm.ReadN(n)
			if err != nil { // ReadN only returns io.EOF
				j.Logger().Trace().Msgf("L4 client ReadN ending (err = %+v)", err)
				j.wm.Done = true
				return 0, err
			}
			nw, err := writer.Write(chunk[:min(j.wm.WriteStepSize, nn)])
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
		var reader io.Reader = conn
		if j.Schedule != nil {
			reader = pacing.NewPacedReader(ctx, conn, j.Schedule)
		}

		iocmd = func(n int64) (int64, error) {
			nw, err := reader.Read(chunk[:j.wm.ReadStepSize])
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

	j.Logger().Info().Msgf("L4 client %s %d bytes remote %s", cmd, j.ContentSize, j.targetURL)
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
				j.wm.Error = fmt.Errorf("conn io error: %w %s", err, reflect.TypeOf(err))
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
				// quic generates  context.DeadlineExceeded
				if errors.Is(err, context.DeadlineExceeded) {
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
