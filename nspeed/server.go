package nspeed

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nspeed/metrics"

	"github.com/rs/zerolog"
	"golang.org/x/exp/slices"
	"nspeed.app/crypto"
	"nspeed.app/humanize"
	"nspeed.app/iobuffer"
	"nspeed.app/network"
)

type TLSSettings struct {
	ServeTLS         bool
	UseTLSSelfConfig bool // if true ignore TLSCertFile/TLSKeyFile use TLSSelfConfig
	TLSCertFile      string
	TLSKeyFile       string
	SkipTLSCheck     bool
}

func (s *TLSSettings) MakeTLSConfig() (*tls.Config, error) {
	if s.UseTLSSelfConfig {
		return crypto.GetTLSSelfConfig(), nil
	} else if s.ServeTLS {
		// todo: this should be done earlier may be ?
		cer, err := tls.LoadX509KeyPair(s.TLSCertFile, s.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("LoadX509KeyPair: %w", err)
		}
		return &tls.Config{Certificates: []tls.Certificate{cer}}, nil
	}
	return nil, nil
}

// parameters of a server endpoint
// todo: rename & refactor ?
type HostParameters struct {
	TLSSettings
	Protocol    string
	Host        string
	Port        int
	IPVersion   network.IPVersion // 0 (use global setting), 4 (force IPv4)  or 6 (force IPv6)
	MaxBodySize int64             // max inbound size and outbound size for streaming (not fileserve)- todo: split into 2 distinct settings?
	MaxDuration time.Duration
	IdleTimeout time.Duration
	H2C         bool
	MTU         uint16
	HTTP11      bool
	HTTP2       bool
	HTTP3       bool
	// AltSvc      string // TODO
	// UseTLSSelfConfig bool // if true ignore TLSCertFile/TLSKeyFile use TLSSelfConfig
	// TLSCertFile      string
	// TLSKeyFile       string
	RunDuration time.Duration
	MaxRequests int
	Directory   string // directory to serve content from /dir url
	API         bool
	APIOnly     bool
}

// allowed protocols. todo: make a type ?
var Protocols = []string{"HTTP", "UDP", "TCP", "QUIC", "QUIC-GO"}

func validProtocol(proto string) bool {
	return slices.Contains(Protocols, proto)
}

// initialize a new HostParameters
func NewHostParameters() *HostParameters {
	return &HostParameters{
		Protocol:    "HTTP",
		MaxBodySize: DefaultMaxBodySize,
		MaxDuration: DefaultMaxDuration,
		IdleTimeout: DefaultIdelTimeout,
	}
}

func CmdServerParse(command string, cmdargs []string, jl *JobList, output io.Writer) ([]string, error) {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(output)
	host := NewHostParameters()
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage of %s: ", serveCommand)
		fmt.Fprintf(fs.Output(), "  %s [options]\n", serveCommand)
		fmt.Fprintf(fs.Output(), "\nAvailable options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(fs.Output(), "\n")
	}

	fs.StringVar(&host.Protocol, "P", "http", "communication protocol")
	fs.StringVar(&host.Host, "a", "localhost", "Hostname or ip address to listen (use \"\" for all interfaces/all addresses)")
	//flag.Lookup("address").NoOptDefVal = ""
	useIPv4 := fs.Bool("4", false, "use IPv4 only")
	useIPv6 := fs.Bool("6", false, "use IPv6 only")
	fs.IntVar(&host.Port, "p", 0, "Port number to listen on")
	maxsize := fs.String("s", humanize.FormatByteDecimalUnits(DefaultMaxBodySize), "Maximum size of response, in bytes or units of bytes")
	maxtime := fs.Uint("m", uint(DefaultMaxDuration.Seconds()), "Maximum duration in seconds a response may take")
	fs.DurationVar(&host.RunDuration, "d", 0, "duration after which the server shutdown (can use duration units: s,m,h)")
	fs.IntVar(&host.MaxRequests, "n", 0, "number of requests after which  the server shutdown (0 = run forever)")
	fs.StringVar(&host.TLSCertFile, "cert", "", "TLS certificate file")
	fs.StringVar(&host.TLSKeyFile, "key", "", "TLS Key file")
	fs.BoolVar(&host.UseTLSSelfConfig, "self", false, "listen in HTTPS using a self-signed TLS certificate")
	fs.StringVar(&host.Directory, "dir", "", "directory to serve content from (url will be /dir)")
	fs.BoolVar(&host.H2C, "h2c", false, "listen in HTTP/2 Cleartext (H2C) mode")
	mtu := fs.Uint("mtu", 0, "MTU to use (80 will be added)")
	fs.BoolVar(&host.HTTP11, "http1.1", false, "listen only in HTTP/1.1 (this will disable HTTP/2) - implies -self is no cert/key")
	fs.BoolVar(&host.HTTP2, "http2", false, "listen only in HTTP/2 (this will disable HTTP/1.1) - implies -self is no cert/key")
	fs.BoolVar(&host.HTTP3, "http3", false, "also listen using HTTP/3 on QUIC (udp - same port) - EXPERIMENTAL")
	fs.BoolVar(&host.API, "api", false, "enable API endpoint (activate /api url)")
	// fs.StringVar(&host.AltSvc, "alt", "", "add alt-svc endpoints (use id names, comma separated)") NYI
	id := fs.String("id", "", "unique ID/name of this server")
	if err := fs.Parse(cmdargs); err != nil {
		return cmdargs, err
	}

	// proto, we keep the passed capitalization tilazation in the error to not confuse the user
	if !validProtocol(strings.ToUpper(host.Protocol)) {
		return cmdargs, fmt.Errorf("unknown protocol: %s", host.Protocol)
	}
	host.Protocol = strings.ToUpper(host.Protocol)

	//
	host.MaxDuration = time.Duration(*maxtime) * time.Second

	// force IPv4 or IPv6
	if *useIPv4 && *useIPv6 {
		fs.Usage()
		return cmdargs, errors.New("specify IPv4 or IPv6 but not both")
	}
	if *useIPv4 {
		host.IPVersion = 4
	}
	if *useIPv6 {
		host.IPVersion = 6
	}

	ms, err := humanize.ParseByteUnits(*maxsize)
	if err != nil {
		fs.Usage()
		return cmdargs, err
	}
	host.MaxBodySize = int64(ms)

	host.ServeTLS = host.UseTLSSelfConfig
	if host.TLSCertFile != "" || host.TLSKeyFile != "" {
		if _, err := os.Stat(host.TLSCertFile); os.IsNotExist(err) {
			return cmdargs, errors.New("bad or missing TLS certificate file: " + host.TLSCertFile)
		}
		if _, err := os.Stat(host.TLSKeyFile); os.IsNotExist(err) {
			return cmdargs, errors.New("bad or missing TLS key file: " + host.TLSKeyFile)
		}
		host.ServeTLS = true
	}

	if (host.HTTP11 || host.HTTP2 || host.HTTP3) && host.Protocol != "HTTP" {
		return cmdargs, errors.New("specify a http version requires http protocol")
	}

	// force -self if http2 or http3 or quic/quic-go and no tls files
	if (host.HTTP11 || host.HTTP2 || host.HTTP3 || host.Protocol == "QUIC" || host.Protocol == "QUIC-GO") && !host.ServeTLS {
		host.UseTLSSelfConfig = true
		host.ServeTLS = true
	}
	if host.HTTP11 && host.HTTP2 {
		return cmdargs, errors.New("can't use both -http1.1 and -http2")
	}

	// if encryption is on (-self, -http1.1, -http2, -http3 or -key/-cert) then don't allow h2c
	if host.ServeTLS && host.H2C {
		return cmdargs, errors.New("h2c not compatible with self/http1.1/http2/http3" + host.TLSKeyFile)
	}

	// MTU
	if mtu != nil && *mtu > 0 && *mtu < 1200 && host.IPVersion == 6 {
		return cmdargs, fmt.Errorf("minimum MTU for IPv6 is 1200(+80)")
	}
	host.MTU = uint16(*mtu)

	args := fs.Args()
	var j Job
	switch host.Protocol {
	case "HTTP":
		// fmt.Println("CREATING L7 SERVER", host.Protocol)
		j = &JobHTTPServer{Parameters: host, JobList: jl}
	case "TCP", "UDP", "QUIC", "QUIC-GO":
		// fmt.Println("CREATING L4 SERVER", host.Protocol)
		j = &L4Server{Parameters: host, JobList: jl}
	default:
		return args, fmt.Errorf("unimplemented protocol: %s", host.Protocol)
	}
	j.SetID(*id)
	err = jl.AddJobs(j)
	return args, err
}

// hosting a L4 server (tcp,udp, or quic)
type L4Server struct {
	JobData
	Parameters    *HostParameters
	Cancel        context.CancelFunc
	TotalRequests atomic.Uint64
	addr          string
	//listener      *net.Listener
	// udpConn       *net.UDPConn
	tlsConfig *tls.Config
	// allow extension
	JobList *JobList
	// wm      *metrics.Metric
	URL *url.URL
}

func (j *L4Server) GetURL() *url.URL {
	return j.URL
}

// Job interface - see job.go
//   GetTarget() string
//   GoString() string
//   PrintString(bool, bool) string
//   IsInfinite() bool
//   Init() error
//   Run(context.Context, *sync.WaitGroup) (*metrics.Metric, error)

func (j *L4Server) GetTarget() string {
	return fmt.Sprintf("%s server %s:%d (IP version: %d)", j.Parameters.Protocol, j.Parameters.Host, j.Parameters.Port, j.Parameters.IPVersion)
}

func (j *L4Server) GoString() string {
	if zerolog.GlobalLevel() > zerolog.InfoLevel {
		return fmt.Sprintf("%s server %s:%d", j.Parameters.Protocol, j.Parameters.Host, j.Parameters.Port)
	}
	return fmt.Sprintf("%s server {Address: %q, port: %d, IPversion: %d, max_size: %s, max_request, max duration: %v, UseTLS: %t, maxruns: %d, max life: %v}",
		j.Parameters.Protocol,
		j.Parameters.Host,
		j.Parameters.Port,
		j.Parameters.IPVersion,
		humanize.ByteCountDecimal(j.Parameters.MaxBodySize)+"B",
		j.Parameters.MaxDuration,
		j.Parameters.ServeTLS,
		j.Parameters.MaxRequests,
		j.Parameters.RunDuration,
	)
}

func (j *L4Server) PrintString(_, _ bool) string {
	return j.Parameters.Protocol + " server " + j.ID()
}

func (j *L4Server) IsInfinite() bool {
	return true
}

func (j *L4Server) Init(opt Options) error {

	j.TotalRequests.Store(0)

	ipVersion := opt.IPVersion
	if j.Parameters.IPVersion == 4 || j.Parameters.IPVersion == 6 {
		ipVersion = j.Parameters.IPVersion
	}

	host := j.Parameters.Host
	hostAddr, err := network.InterfaceAddress(j.Parameters.Host, ipVersion)
	if err != nil {
		return fmt.Errorf("InterfaceAddress error: %w", err)
	}
	if hostAddr != nil {
		host = hostAddr.IP.String()
	}

	j.addr = net.JoinHostPort(host, strconv.Itoa(j.Parameters.Port))

	j.Logger().Debug().Msgf("address parameters gave:'%s' (from %s)", j.addr, hostAddr)

	j.tlsConfig, err = j.Parameters.MakeTLSConfig()
	if err != nil {
		j.Logger().Error().Err(err).Msgf("MakeTLSConfig error")
		return fmt.Errorf("MakeTLSConfig error: %w", err)
	}
	return nil
}

func (j *L4Server) Run(ctx context.Context, wg *sync.WaitGroup, _ Options) (*metrics.Metric, error) {

	ctx, j.Cancel = context.WithCancel(ctx)
	if j.Parameters.RunDuration != 0 {
		ctx2, cancel := context.WithTimeout(ctx, j.Parameters.RunDuration)
		defer cancel()
		ctx = ctx2
		j.Logger().Debug().Msgf("server will shutdown in: %v", j.Parameters.RunDuration)
	}

	L3Proto := strings.ToLower(j.Parameters.Protocol)
	if j.Parameters.Protocol == "QUIC" || j.Parameters.Protocol == "QUIC-GO" {
		L3Proto = "udp"
	}
	network := network.AddIPVersionToNetwork(L3Proto, j.Parameters.IPVersion)

	if j.Parameters.Protocol == "TCP" {
		return j.ServeTCP(ctx, network, wg)
	}
	if j.Parameters.Protocol == "QUIC-GO" {
		return j.ServeQUICGO(ctx, network, wg)
	}
	if j.Parameters.Protocol == "QUIC" {
		return j.ServeQUIC(ctx, network, wg)
	}
	defer wg.Done()
	return nil, fmt.Errorf("protocol %s not yet implemented", j.Parameters.Protocol)
}

func (j *L4Server) ServeTCP(ctx context.Context, network string, wg *sync.WaitGroup) (*metrics.Metric, error) {
	// TCP server

	// this avoid calling wg.Done() twice
	done := false
	defer func() {
		if !done {
			wg.Done()
		}
	}()

	var ls net.Listener
	var err error
	if j.tlsConfig == nil {
		lc := net.ListenConfig{}
		ls, err = lc.Listen(ctx, network, j.addr)
	} else {
		ls, err = tls.Listen(network, j.addr, j.tlsConfig)
	}

	if err != nil {
		return nil, fmt.Errorf("listen failed: %s (%s)", err, j.addr)
	}
	//j.listener = &ls

	acceptDone := make(chan error)
	tls := ""
	if j.tlsConfig != nil {
		tls = "(tls)"
	}
	j.URL = &url.URL{Scheme: strings.ToLower(j.Parameters.Protocol), Host: ls.Addr().String()}
	j.Logger().Warn().Msgf("L4 %s server listening %s %s (URL = %s)", j.Parameters.Protocol, ls.Addr(), tls, j.URL)

	// signal we're ready  to accept connections
	wg.Done()
	done = true

	//spawn the accept loop
	//result := make(chan metrics.Metric)
	var running atomic.Bool
	running.Store(true)
	go func() {
		for {
			conn, err := ls.Accept()
			if err != nil {
				j.Logger().Debug().Err(err).Msgf("accept failed with:")
				if running.Load() {
					acceptDone <- err
				} else {
					acceptDone <- nil
				}
				break
			}
			j.TotalRequests.Add(1)
			go j.handleConnection(conn)
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
	_ = ls.Close()
	// wait for sub go routine (ls.Accept should unblock with an "use of closed network connection") to end
	<-acceptDone
	j.Logger().Debug().Err(err).Msgf("L4 server ended")
	return &metrics.Metric{}, err
}

// MT
func (j *L4Server) handleConnection( /*ctx context.Context,*/ conn net.Conn /*result chan metric.Metric*/) {
	defer conn.Close()

	j.Logger().Warn().Msgf("incoming connection from %s", conn.RemoteAddr())

	var chunkSize int64 = 1 * 1024 * 1024 // todo: move this in a parameter/const, have the client send it eventually or be adaptative?
	chunk := iobuffer.GetChunk(chunkSize)

	wm := metrics.Metric{}
	wm.InitNew()
	wm.Command = fmt.Sprintf("%s with %s", j.GetTarget(), conn.RemoteAddr())
	err := conn.SetDeadline(time.Now().Add(j.Parameters.MaxDuration))
	if err != nil {
		j.logger.Error().Err(err).Msgf("SetDeadline failed")
		return
	}

	// read message
	msg, err := ReadL4Message(conn)
	j.logger.Info().Msgf("SERVER GOT MESSAGE: %+v (err=%+v)", msg, err)
	if err != nil {
		j.logger.Error().Err(err).Msgf("read message failed")
		return
	}
	wm.Remote, err = netip.ParseAddrPort(conn.RemoteAddr().String())
	if err != nil {
		j.logger.Debug().Err(err).Msgf("error parsing remote address: %s", conn.RemoteAddr().String())
	}
	wm.Protocol = j.Parameters.Protocol

	if msg.Command == "PUT" {
		j.logger.Info().Msgf("L4 server started receiving loop for %d bytes", msg.Size)
		for {
			n, err := conn.Read(chunk)
			if err != nil {
				wm.Cancelled = true
				if err != io.EOF {
					wm.Error = err
					wm.Cancelled = false
				}
				break
			}
			wm.WriteN(int64(n))
		}
		wm.Valid = true
	}
	if msg.Command == "GET" {
		j.logger.Info().Msgf("L4 server started sending loop for %d bytes", msg.Size)
		// wm will produce ContentWriteSize starting from TotalWrite with chunks of size wm.WriteStepSize
		wm.ContentWriteSize = msg.Size
		wm.TotalWrite = 0
		wm.WriteStepSize = chunkSize
		for {
			n, err := wm.ReadN(chunkSize)
			if err != nil {
				wm.Valid = true
				break
			}
			if n == 0 {
				wm.Valid = true
				break
			}
			j.logger.Trace().Msgf("L4 server sending %d bytes", n)
			_, err = conn.Write(chunk[:n])
			if err != nil {
				wm.Cancelled = true
				if err != io.EOF {
					wm.Error = err
					wm.Cancelled = false
				} else {
					wm.Valid = true
				}
				break
			}
		}
		if wm.Error == nil {
			err = FlushConn(conn)
			j.logger.Trace().Msg("L4 flushing")
			if err != nil {
				j.logger.Warn().Err(err).Msg("error flushing")
			}
		}
	}
	j.logger.Debug().Msgf("L4 server ended loop (%t) (%s)", wm.Cancelled, wm.Error)
	wm.LogFinal(j.Logger())
	j.Logger().Debug().Msgf("L4 handleConnection ended")
}
