package nspeed

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/exp/slices"
	"nspeed.app/metric"
	"nspeed.app/utils"
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
		if TLSSelfConfig == nil {
			// this shouldn't happen if parameters check are done correctly
			return nil, fmt.Errorf("no self certificate installed")
		}
		return TLSSelfConfig.Clone(), nil
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
	IPVersion   utils.IPVersion // 0 (use global setting), 4 (force IPv4)  or 6 (force IPv6)
	MaxBodySize int64           // max inbound size and outbound size for streaming (not fileserve)- todo: split into 2 distinct settings?
	MaxDuration time.Duration
	IdleTimeout time.Duration
	H2C         bool
	HTTP11      bool
	HTTP2       bool
	HTTP3       bool
	AltSvc      string
	// UseTLSSelfConfig bool // if true ignore TLSCertFile/TLSKeyFile use TLSSelfConfig
	// TLSCertFile      string
	// TLSKeyFile       string
	RunDuration time.Duration
	MaxRequests int
	Directory   string // directory to serve content from /dir url
	API         bool
	APIOnly     bool
}

var Protocols = []string{"http", "udp", "tcp", "quic", "quic-go"}

func validProtocol(proto string) bool {
	return slices.Contains(Protocols, proto)
}

// initialize a new HostParameters
func NewHostParameters() *HostParameters {
	return &HostParameters{
		Protocol:    "http",
		MaxBodySize: DefaultMaxBodySize,
		MaxDuration: DefaultMaxDuration,
		IdleTimeout: DefaultIdelTimeout,
	}
}

func ServerParse(command string, cmdargs []string, jl *JobList, output io.Writer) ([]string, error) {
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

	fs.StringVar(&host.Protocol, "P", "http", "comunication protocol")
	fs.StringVar(&host.Host, "a", "localhost", "Hostname or ip address to listen (use \"\" for all interfaces/all addresses)")
	//flag.Lookup("address").NoOptDefVal = ""
	useIPv4 := fs.Bool("4", false, "use IPv4 only")
	useIPv6 := fs.Bool("6", false, "use IPv6 only")
	fs.IntVar(&host.Port, "p", 0, "Port number to listen on")
	maxsize := fs.String("s", utils.FormatByteDecimalUnits(DefaultMaxBodySize), "Maximum size of response, in bytes or units of bytes")
	maxtime := fs.Uint("m", uint(DefaultMaxDuration.Seconds()), "Maximum duration in seconds a response may take")
	fs.DurationVar(&host.RunDuration, "d", 0, "duration after which the server shutdown (can use duration units: s,m,h)")
	fs.IntVar(&host.MaxRequests, "n", 0, "number of requests after which  the server shutdown (0 = run forever)")
	fs.StringVar(&host.TLSCertFile, "cert", "", "TLS certificate file")
	fs.StringVar(&host.TLSKeyFile, "key", "", "TLS Key file")
	fs.BoolVar(&host.UseTLSSelfConfig, "self", false, "listen in HTTPS using a self-signed TLS certificate")
	fs.StringVar(&host.Directory, "dir", "", "directory to serve content from (url will be /dir)")
	fs.BoolVar(&host.H2C, "h2c", false, "listen in HTTP/2 Cleartext (H2C) mode")
	fs.BoolVar(&host.HTTP11, "http1.1", false, "listen only in HTTP/1.1 (this will disable HTTP/2) - implies -self is no cert/key ")
	fs.BoolVar(&host.HTTP2, "http2", false, "listen only in HTTP/2 (this will disable HTTP/1.1) - implies -self is no cert/key ")
	fs.BoolVar(&host.HTTP3, "http3", false, "also listen using HTTP/3 on QUIC (udp - same port) - EXPERIMENTAL")
	fs.BoolVar(&host.API, "api", false, "enable API endpoint (activate /api url)")
	fs.StringVar(&host.AltSvc, "alt", "", "add alt-svc endpoints (use id names, comma separated)")
	id := fs.String("id", "", "unique ID/name of this server")
	if err := fs.Parse(cmdargs); err != nil {
		return cmdargs, err
	}

	if !validProtocol(host.Protocol) {
		return cmdargs, fmt.Errorf("invalid protocol: %s", host.Protocol)
	}
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

	ms, err := utils.ParseByteUnits(*maxsize)
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

	if (host.HTTP11 || host.HTTP2 || host.HTTP3) && host.Protocol != "http" {
		return cmdargs, errors.New("specify a http version requires http")
	}

	// force -self if http2 or http3 and no tls files
	if (host.HTTP11 || host.HTTP2 || host.HTTP3) && !host.ServeTLS {
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

	args := fs.Args()
	var j Job
	switch host.Protocol {
	case "http":
		j = &JobHTTPServer{Parameters: host, JobList: jl}
	case "tcp", "udp", "quic", "quic-go":
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
	listener      *net.Listener
	// udpConn       *net.UDPConn
	tlsConfig *tls.Config
	// allow extension
	JobList *JobList
	// wm      *metric.Metric
}

// Job interface - see job.go
//   GetTarget() string
//   GoString() string
//   PrintString(bool, bool) string
//   IsInfinite() bool
//   Init() error
//   Run(context.Context, *sync.WaitGroup) (*metric.Metric, error)

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
		utils.ByteCountDecimal(j.Parameters.MaxBodySize)+"B",
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

func (j *L4Server) Init() error {

	j.TotalRequests.Store(0)

	ipVersion := GlobalOptions.IPVersion
	if j.Parameters.IPVersion == 4 || j.Parameters.IPVersion == 6 {
		ipVersion = j.Parameters.IPVersion
	}

	host := j.Parameters.Host
	hostAddr, err := utils.InterfaceAddress(j.Parameters.Host, ipVersion)
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

func (j *L4Server) Run(ctx context.Context, wg *sync.WaitGroup) (*metric.Metric, error) {

	done := false
	defer func() {
		if !done {
			wg.Done()
		}
	}()

	ctx, j.Cancel = context.WithCancel(ctx)
	if j.Parameters.RunDuration != 0 {
		ctx2, cancel := context.WithTimeout(ctx, j.Parameters.RunDuration)
		defer cancel()
		ctx = ctx2
		j.Logger().Debug().Msgf("server will shutdown in: %v", j.Parameters.RunDuration)
	}

	network := utils.AddIPVersionToNetwork("tcp", j.Parameters.IPVersion, GlobalOptions.IPVersion)

	var ls net.Listener

	lc := net.ListenConfig{}
	var err error
	if j.tlsConfig != nil {
		ls, err = tls.Listen(network, j.addr, j.tlsConfig)
	} else {
		ls, err = lc.Listen(ctx, network, j.addr)
	}

	if err != nil {
		return nil, fmt.Errorf("listen failed: %s (%s)", err, j.addr)
	}
	j.listener = &ls

	acceptDone := make(chan error)
	tls := ""
	if j.tlsConfig != nil {
		tls = "(tls)"
	}
	j.Logger().Warn().Msgf("server %s listening %s %s", j.Parameters.Protocol, ls.Addr(), tls)
	//spawn the accept loop
	//result := make(chan metric.Metric)
	go func() {
		for {
			conn, err := ls.Accept()
			if err != nil {
				j.Logger().Debug().Err(err).Msgf("accept failed")
				acceptDone <- err
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
		err = ctx.Err()
	}
	j.Logger().Debug().Err(ctx.Err()).Msgf("ending")

	//todo: handle pending connections
	ls.Close()

	return nil, err
}

// MT
func (j *L4Server) handleConnection( /*ctx context.Context,*/ conn net.Conn /*result chan metric.Metric*/) {
	defer conn.Close()

	j.Logger().Debug().Msgf("incoming connection from %s", conn.RemoteAddr())

	var chunkSize int64 = 4 * 1024 * 1024 // todo: move this in a parameter/const, have the client send it eventually or be adaptative?
	chunk := utils.GetChunk(chunkSize)

	wm := metric.Metric{}
	wm.InitNew()
	wm.Command = fmt.Sprintf("%s with %s", j.GetTarget(), conn.RemoteAddr())
	err := conn.SetDeadline(time.Now().Add(j.Parameters.MaxDuration))
	if err != nil {
		j.logger.Error().Err(err).Msgf("SetDeadline failed")
	}
	for {
		n, err := conn.Read(chunk)
		if err != nil {
			if err != io.EOF {
				wm.Error = err
			}
			break
		}
		wm.WriteN(int64(n))
	}
	wm.Valid = true
	wm.PrintFinal(os.Stdout)
	j.Logger().Warn().Msgf("NYI sending or receiving")
}
