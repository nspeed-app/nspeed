package nspeed

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/quic-go/qlog"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"nspeed.app/utils"
)

var ServerName string = "NSpeed"
var TLSSelfConfig *tls.Config

// Default configuration values
const (
	DefaultMaxBodySize int64 = 1000 * 1000 * 1000 * 40 // bytes
	DefaultMaxDuration       = 10 * time.Second
	DefaultPort              = 7333
	DirPath                  = "/dir/"
)

// parameters of a server endpoint
// todo: rename & refactor ?
type HostParameters struct {
	Host             string
	Port             int
	ForceIPVersion   int   // 0 (none), 4 (force IPv4)  or 6 (force IPv6)
	MaxBodySize      int64 // max inbound size and outbound size for streaming (not fileserve)- todo: split into 2 distinct settings?
	MaxDuration      time.Duration
	ServeTLS         bool
	H2C              bool
	HTTP11           bool
	HTTP2            bool
	HTTP3            bool
	UseTLSSelfConfig bool // if true ignore TLSCertFile/TLSKeyFile use TLSSelfConfig
	TLSCertFile      string
	TLSKeyFile       string
	RunDuration      time.Duration
	MaxRequests      int
	Directory        string // directory to serve content from /dir url
	API              bool
	APIOnly          bool
	ID               string
}

// initialize a new HostParameters
func NewHostParameters() *HostParameters {
	return &HostParameters{
		Port:        DefaultPort,
		MaxBodySize: DefaultMaxBodySize,
		MaxDuration: DefaultMaxDuration,
	}
}

// the HTTP Server
// implements Job interface
type JobHTTPServer struct {
	Parameters       *HostParameters
	Server           *http.Server
	Mux              *http.ServeMux
	Cancel           context.CancelFunc
	Close            func(t time.Duration)
	Logger           *zerolog.Logger
	StreamPathRegexp *regexp.Regexp
	TotalRequests    int
	mutex            sync.Mutex
	api              *APIHost
	addr             string
	URL              string
	listener         *net.Listener
	udpConn          *net.UDPConn
}

func (j *JobHTTPServer) GetTarget() string {
	return fmt.Sprintf("server %s:%d (IP version: %d)", j.Parameters.Host, j.Parameters.Port, j.Parameters.ForceIPVersion)
}

func (j *JobHTTPServer) Expand(jobs []Job, group int) []Job {
	return append(jobs, j)
}

func (j *JobHTTPServer) GoString() string {
	if zerolog.GlobalLevel() > zerolog.InfoLevel {
		return fmt.Sprintf("HTTP server %s:%d", j.Parameters.Host, j.Parameters.Port)
	}
	return fmt.Sprintf("HTTP server {Address: %q, port: %d, IPversion: %d, max_size: %s, max_request, max duration: %v, UseTLS: %t, H2C: %t, maxruns: %d, max life: %v}",
		j.Parameters.Host,
		j.Parameters.Port,
		j.Parameters.ForceIPVersion,
		utils.ByteCountDecimal(j.Parameters.MaxBodySize)+"B",
		j.Parameters.MaxDuration,
		j.Parameters.ServeTLS,
		j.Parameters.H2C,
		j.Parameters.MaxRequests,
		j.Parameters.RunDuration,
	)
}

func (j *JobHTTPServer) PrintString(verbose bool) string {
	return "SERVER"
}

func (j *JobHTTPServer) GetMetric() (NetMetric, error) {
	return NetMetric{}, fmt.Errorf("not yet implemented")
}

func (j *JobHTTPServer) IsInfinite() bool {
	if j.Parameters.MaxRequests > 0 {
		return false
	}
	if j.Parameters.RunDuration > 0 {
		return false
	}
	return true
}

// called each time before a request, prevent new request before actual shutdown if MaxRequest reached
// return false if too many requests
func (j *JobHTTPServer) OnRequestStart() bool {
	j.mutex.Lock()
	defer j.mutex.Unlock()

	j.TotalRequests++
	done := j.Parameters.MaxRequests > 0 && j.TotalRequests > j.Parameters.MaxRequests

	if done {
		j.Logger.Info().Msgf("%d max total requests reached", j.TotalRequests)
		return false
	}
	return true
}

// called each time a request is completed, see muxer
func (j *JobHTTPServer) OnRequestEnd() {
	j.mutex.Lock()
	defer j.mutex.Unlock()
	done := j.Parameters.MaxRequests > 0 && j.TotalRequests >= j.Parameters.MaxRequests
	if done {
		j.Logger.Info().Msgf("%d max total requests reached", j.TotalRequests)
		j.Cancel()
	}
}

func notImplemented(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// CreateHandler returns an http.Handler that exposes all the server endpoints
func (h *JobHTTPServer) CreateHandler() http.Handler {
	mux := http.NewServeMux()
	h.Mux = mux

	// serve static files h.Parameters.Directory
	if h.Parameters.Directory != "" {
		log.Info().Msgf("serving directory '%s' at %s", h.Parameters.Directory, DirPath)
		mux.Handle(DirPath, http.StripPrefix(DirPath, http.FileServer(http.Dir(h.Parameters.Directory))))
	}

	if !h.Parameters.APIOnly {
		mux.HandleFunc("/", h.rootHandler)
	}

	mux.HandleFunc("/ip", notImplemented)

	// mux and chain of middlewares (see http_middleware.go)
	var handler http.Handler
	handler = mux
	handler = limitRequestSize(h.Parameters.MaxBodySize, handler)
	handler = preflight(handler)
	handler = limitNumRequests(h.OnRequestStart, h.OnRequestEnd, handler)
	if h.Logger != nil {
		handler = logger(h.Logger, handler)
	}

	return handler
}

// handle '/' .. get : stream bytes, post/put - upload bytes
func (h *JobHTTPServer) rootHandler(w http.ResponseWriter, r *http.Request) {
	method := r.Method
	if method == "" {
		method = "GET"
	}

	log.Info().Msgf("rootHandler: %s - %s", method, r.URL.Path)

	if method == "GET" {
		match := h.StreamPathRegexp.FindStringSubmatch(r.URL.Path[1:])
		log.Debug().Strs("match", match).Send()
		// keep this test in sync with the regexp
		if len(match) == 3 {
			h.StreamBytes(w, r, match[1], match[2])
			return
		} else {
			http.Error(w, "Not found (no regexp match)", http.StatusNotFound)
			return
		}
	}

	if method == "POST" && r.URL.Path == "/" {
		h.Upload(w, r)
		return
	}
	//todo: may be only accept some specific path.
	if method == "PUT" {
		h.Upload(w, r)
		return
	}
	//http.NotFound(w, r)
}

// StreamBytes sends 'size' bytes using streaming (io level streaming not HTTP level).
func (h *JobHTTPServer) StreamBytes(w http.ResponseWriter, r *http.Request, size string, extension string) {

	n, err := utils.ParseByteUnits(size)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	numBytes := int64(n)
	log.Info().Msgf("streaming %s bytes (= %d bytes) with extention: %s", size, numBytes, extension)

	if numBytes < 1 {
		numBytes = 1
	} else if numBytes > h.Parameters.MaxBodySize {
		//http.Error(w, "too big", http.StatusBadRequest)
		// we use 413 instead of 404 for now.
		http.Error(w, "too big", http.StatusRequestEntityTooLarge)
		return
	}

	// keep multplt of 16384
	var chunkSize int64 = 256 * 1024 // todo: move this in a parameter/const, have the client send it eventually or be adaptative?

	//danger zone - the cient is setting the chunk_size - be careful
	// todo: do we still need to support this parameter ?
	if r.URL.Query().Get("chunk_size") != "" {
		cs, err := utils.ParseByteUnits(r.URL.Query().Get("chunk_size"))
		chunkSize = int64(cs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if chunkSize == 0 {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// todo- for now we limit chunkSize to maxBodySize. do we need a new cmd line parameter for max chunk size ?
		// since huge chunksize can overload the server memory this is not safe
		if chunkSize > MaxChunkSize {
			http.Error(w, fmt.Sprintf("chunk size too big (max allowed is %d)", MaxChunkSize), http.StatusBadRequest)
			return
		}
		log.Info().Msgf("chunk_size set to %d", chunkSize)
	}
	// end danger zone

	chunk := BigChunk[:chunkSize]

	// content-type
	w.Header().Set("Content-Type", "application/octet-stream")
	ct := mime.TypeByExtension(extension)
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	pct := r.URL.Query().Get("ct")
	if pct != "" {
		_, _, err := mime.ParseMediaType(pct)
		if err != nil {
			http.Error(w, fmt.Sprintf("unknown content type: %s", pct), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", pct)
	}

	// content-length
	w.Header().Set("Content-Length", strconv.FormatInt(numBytes, 10))

	// status
	w.WriteHeader(http.StatusOK) // this could also be done by the first w.Write()

	// prepare metrics handler
	wm := Metrics{
		MetricsData: MetricsData{
			Done:    false,
			Expired: false,
			Target:  fmt.Sprintf("download %s to %s (chunk=%d)", size, r.RemoteAddr, chunkSize),
			Job:     h,
		},
	}
	wm.TransfertStartTime = time.Now()
	wm.FirstResponseByte = wm.TransfertStartTime
	wm.ContentWriteSize = numBytes //todo: investigate what happens when timed out
	//f := w.(http.Flusher)
	// send the buffer as many times as needed --> todo: why don't we io.copy to a wm like the client ?
	var numChunk = numBytes / chunkSize
	wm.StepSize = chunkSize
	for i := int64(0); i < numChunk; i++ {
		n, err := w.Write(chunk)
		wm.TotalWrite += int64(n)
		wm.ElapsedTime = time.Since(wm.TransfertStartTime)
		if err != nil {
			wm.Error = err
			wm.Expired = true
			break
		}
		wm.WriteCount++
	}

	// send the modulo is any or not interrupted/timed out
	// fmt.Println("modulo :", numBytes % chunkSize)
	if numBytes%chunkSize > 0 && !wm.Expired {
		n, err := w.Write(chunk[:numBytes%chunkSize])
		//f.Flush()
		wm.TotalWrite += int64(n)
		wm.ElapsedTime = time.Since(wm.TransfertStartTime)
		if err != nil {
			wm.Error = err
			wm.Expired = true
		}
	}
	wm.Done = true
	wm.Valid = true
	wm.LogFinal(h.Logger)
}

// handle upload
func (h *JobHTTPServer) Upload(w http.ResponseWriter, r *http.Request) {
	h.Logger.Info().Msgf("starting Upload (%s) from %s (%d)", r.Method, r.RemoteAddr, r.ContentLength)
	wm := Metrics{
		MetricsData: MetricsData{
			Done:    false,
			Expired: false,
			Target:  fmt.Sprintf("upload from %s", r.RemoteAddr),
			Job:     h,
		},
	}
	wm.FirstResponseByte = time.Now()

	n, err := io.CopyBuffer(io.Discard, io.TeeReader(r.Body, &wm), BigBuffer)
	wm.ElapsedTime = time.Since(wm.FirstResponseByte)
	wm.TotalRead = n
	wm.Error = err
	wm.Valid = true
	wm.LogFinal(h.Logger)
}

// implements the http server

func (s *JobHTTPServer) Init(wm *Metrics) error {

	// todo: this should be a global and Once
	s.StreamPathRegexp = regexp.MustCompile("^(" + utils.ByteUnitsRegexp + ")(\\.[a-zA-Z0-9]+)?$")

	s.Logger = &wm.Logger //

	s.TotalRequests = 0 // just in case we reuse

	host := s.Parameters.Host

	// empty host is a special case

	s.Logger.Debug().Msgf("host parameter is:'%s'", host)
	if host != "" {
		// if host is an interface name
		hostIPAddrs, err := utils.ResolveInterfaceHostAddress(s.Parameters.Host)
		if err != nil {
			return err
		}

		fhostIPAddrs := utils.FilterAddresses(hostIPAddrs, s.Parameters.ForceIPVersion)

		if len(fhostIPAddrs) == 0 {
			s.Logger.Debug().Msgf("ResolveInterfaceHostAddress/FilterAddresses : %v/%v,%d", hostIPAddrs, fhostIPAddrs, s.Parameters.ForceIPVersion)
			return errors.New("no valid candidate address found")
		}
		host = fhostIPAddrs[0].String()
	}

	// since Addr of http.Server is a single value we only use the first address
	listenAddr := net.JoinHostPort(host, strconv.Itoa(s.Parameters.Port))
	s.Server = &http.Server{
		Addr:         listenAddr,
		Handler:      s.CreateHandler(),
		WriteTimeout: s.Parameters.MaxDuration,
		ReadTimeout:  s.Parameters.MaxDuration,
	}

	if s.Parameters.H2C {
		s.Server.Handler = h2c.NewHandler(s.Server.Handler, &http2.Server{})
	}

	if s.Parameters.API {
		s.api = &APIHost{Parameters: s.Parameters, Server: s}
		return s.api.Init(wm)
	}
	return nil
}
func (s *JobHTTPServer) Run(ctx context.Context, wg *sync.WaitGroup) error {

	ctx, s.Cancel = context.WithCancel(ctx)

	// run duration is either global timeout from ctx or RunDuration parameter
	if s.Parameters.RunDuration != 0 {
		ctx2, cancel := context.WithTimeout(ctx, s.Parameters.RunDuration)
		defer cancel()
		ctx = ctx2
		s.Logger.Debug().Msgf("server will shutdown in: %v", s.Parameters.RunDuration)
	}
	// exitCh will be closed when it is safe to exit, after graceful shutdown
	exitCh := make(chan struct{})

	// call this to shutdown server (net.http version)
	s.Close = func(shutdownTimeout time.Duration) {
		lctx, lcancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer lcancel()

		s.Server.SetKeepAlivesEnabled(false)
		if err := s.Server.Shutdown(lctx); err != nil {
			s.Logger.Debug().Msgf("shutdown error: %s", err)
		}
	}

	go func() {
		<-ctx.Done()
		s.Logger.Debug().Msgf("shutdown started by context : %s", ctx.Err())

		s.Close(s.Parameters.MaxDuration + s.Parameters.MaxDuration*time.Second) //todo check with global timeout too

		close(exitCh)
	}()

	// TLS stuff (todo move this in Init ?)
	if s.Parameters.UseTLSSelfConfig {
		if TLSSelfConfig != nil {
			s.Server.TLSConfig = TLSSelfConfig
		} else {
			// this shouldn't happen if parameters check are done correctly
			s.Logger.Fatal().Msgf("no self certificate installed")
		}
	} else if s.Parameters.ServeTLS {
		// todo: this should be done earlier may be ?
		cer, err := tls.LoadX509KeyPair(s.Parameters.TLSCertFile, s.Parameters.TLSKeyFile)
		if err != nil {
			s.Logger.Fatal().Err(err).Msgf("error loading cert/key files")
		}
		s.Server.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cer}}
	}

	err := s.Listen()
	if err != nil {
		s.Logger.Fatal().Msgf("failed to listen: %s", err)
		return err
	}

	// signal parent we're listening
	wg.Done()

	// Serve: todo
	err = s.Serve()
	if err != nil {
		if err != errors.New("quic: Server closed") { // temporary quic fix
			s.Logger.Warn().Msgf("failed to serve: %s", err)
		}
	}

	// we get here if shutdown or error
	<-exitCh
	if err == nil {
		s.Logger.Info().Msgf("shutdown finished")
	}
	return nil
}

// Listen opens the listening conn
func (s *JobHTTPServer) Listen() error {

	if s.Parameters.HTTP3 {
		return s.HTTP3Listen()
	}

	if s.listener != nil {
		return errors.New("a connection is already listening")
	}

	s.addr = s.Server.Addr
	if s.addr == "" {
		s.addr = ":http"
		if s.Parameters.ServeTLS {
			s.addr = ":https"
		}
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.listener = &ln
	if s.Parameters.ServeTLS {
		s.URL = "https://" + ln.Addr().String()
		s.Logger.Warn().Msgf("server listening on %s (H2C = %t)", s.URL, s.Parameters.H2C)
	} else {
		s.URL = "http://" + ln.Addr().String()
		s.Logger.Warn().Msgf("server listening on %s", s.URL)
	}

	return nil
}

// Serve accepts incoming connections for this server.
func (s *JobHTTPServer) Serve() error {
	err := s.serve()
	if err == http.ErrServerClosed {
		err = nil
	}
	return err
}

// serve implements Serve/ServeTLS based on parameters
func (s *JobHTTPServer) serve() error {

	if s.Parameters.HTTP3 {
		return s.HTTP3Serve()
	}

	if s.listener == nil {
		return errors.New("no listener")
	}

	if s.Parameters.HTTP2 {
		return s.HTTP2Serve()
	} else if s.Parameters.ServeTLS {
		if s.Parameters.HTTP11 {
			// force HTTP1.1 as per https://pkg.go.dev/net/http#pkg-overview
			s.Server.TLSNextProto = map[string]func(*http.Server, *tls.Conn, http.Handler){}
		}
		return s.Server.ServeTLS(*s.listener, "", "")

	} else {
		return s.Server.Serve(*s.listener)
	}

}

func (s *JobHTTPServer) HTTP2Serve() error {
	//return errors.New("enforcing HTTP/2 is not implemented yet")
	return s.HTTP2serve()
	// can't do unless we rewrite Serve because of we can't add
	// our own proto handler for http/1.1 (see )

	fmt.Println("using http/2 only with patched crypto")

	err := http2.ConfigureServer(s.Server, &http2.Server{})

	if err != nil {
		return err
	}

	idx := utils.IndexOf(s.Server.TLSConfig.NextProtos, "http/1.1")
	if idx != -1 {
		log.Warn().Msgf("removing http/1.1 from server.TLSConfig")
		s.Server.TLSConfig.NextProtos = append(s.Server.TLSConfig.NextProtos[:idx], s.Server.TLSConfig.NextProtos[idx+1:]...)
	}

	config := s.Server.TLSConfig.Clone()

	tlsListener := tls.NewListener(*s.listener, config)
	return s.Server.Serve(tlsListener)
}

// HTTP3Listen opens the listening udp conn
func (s *JobHTTPServer) HTTP3Listen() error {

	if s.Server == nil {
		return errors.New("no HTTP server")
	}

	if s.udpConn != nil {
		return errors.New("udp connection already listening")
	}

	networkUdp := "udp"
	if s.Parameters.ForceIPVersion == 4 {
		networkUdp = "udp4"
	}
	if s.Parameters.ForceIPVersion == 6 {
		networkUdp = "udp6"
	}

	// Open the listeners
	udpAddr, err := net.ResolveUDPAddr(networkUdp, s.Server.Addr)
	if err != nil {
		return err
	}
	s.udpConn, err = net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	s.URL = "https://" + s.udpConn.LocalAddr().String()
	s.Logger.Warn().Msgf("server listening on %s (HTTP/3 on UDP)", s.URL)
	return nil
}

func (s *JobHTTPServer) HTTP3Serve() error {

	if s.udpConn == nil {
		return errors.New("no udp connection")
	}
	defer s.udpConn.Close()

	quicConf := &quic.Config{}

	// setup debug trace
	if zerolog.GlobalLevel() <= zerolog.TraceLevel {
		quicConf.Tracer = qlog.NewTracer(utils.QLogTracer(s.Logger))
	}

	quicServer := &http3.Server{
		Addr:       s.Server.Addr,
		TLSConfig:  s.Server.TLSConfig,
		QuicConfig: quicConf,
		Handler:    s.Server.Handler,
	}

	h := s.Server.Handler
	s.Server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := quicServer.SetQuicHeaders(w.Header())
		s.Logger.Debug().Msgf("SetQuicHeaders called")
		if err != nil {
			log.Fatal().Err(err)
		}
		h.ServeHTTP(w, r)
	})

	// this blocks or fail
	s.Logger.Info().Msgf("nspeed serving https://%s (http3 on udp)", s.Server.Addr)

	s.Close = func(shutdownTimeout time.Duration) {
		s.Logger.Debug().Msgf("warning CloseGracefully() NYI for quic server, shuting down with Close() -> expect a fatal error")
		// nyi: quicServer.CloseGracefully(shutdownTimeout)
		err := quicServer.Close()
		log.Info().Err(err).Msg("quicServer Close called")
	}
	return quicServer.Serve(s.udpConn)
}
