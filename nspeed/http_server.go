package nspeed

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/lucas-clemente/quic-go/http3"
	"github.com/lucas-clemente/quic-go/logging"
	"github.com/lucas-clemente/quic-go/qlog"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"nspeed.app/nspeed/internal/utils"
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

// parameters of aa server instance
type HostParameters struct {
	Host             string
	Port             int
	ForceIPVersion   int   // 0 (none), 4 (force IPv4)  or 6 (force IPv6)
	MaxBodySize      int64 // max inbound size and outbound size for streaming (not fileserve)- todo: split into 2 distinct settings?
	MaxDuration      time.Duration
	ServeTLS         bool
	H2C              bool
	HTTP3            bool
	UseTLSSelfConfig bool // if true ignore TLSCertFile/TLSKeyFile use TLSSelfConfig
	TLSCertFile      string
	TLSKeyFile       string
	RunDuration      time.Duration
	MaxRequests      int
	Directory        string // directory to serve content from /dir url
	API              bool
	APIOnly          bool
}

// initialize a new HostParameters
func NewHostParameters() *HostParameters {
	return &HostParameters{
		Port:             DefaultPort,
		MaxBodySize:      DefaultMaxBodySize,
		MaxDuration:      DefaultMaxDuration,
		ServeTLS:         false,
		H2C:              false,
		HTTP3:            false,
		UseTLSSelfConfig: false,
	}
}

// the HTTP Server
// implements Job interface
type JobHTTPServer struct {
	Parameters       *HostParameters
	Server           *http.Server
	Mux              *http.ServeMux
	Cancel           context.CancelFunc
	Logger           *zerolog.Logger
	StreamPathRegexp *regexp.Regexp
	TotalRequests    int
	mutex            sync.Mutex
	api              *APIHost
}

func (j *JobHTTPServer) GetTarget() string {
	return fmt.Sprintf("server %s:%d (IP version: %d)", j.Parameters.Host, j.Parameters.Port, j.Parameters.ForceIPVersion)
}

func (j *JobHTTPServer) Expand(jobs []Job, group int) []Job {
	return append(jobs, j)
}

func (j *JobHTTPServer) GoString() string {
	return fmt.Sprintf("HTTP server {Address: %q, port: %d, IPversion: %d, max_size: %s, max_request, max duration: %v, UseTLS: %t, H2C: %t, maxruns: %d, max life: %v}",
		j.Parameters.Host,
		j.Parameters.Port,
		j.Parameters.ForceIPVersion,
		ByteCountDecimal(j.Parameters.MaxBodySize)+"B",
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

// Handler returns an http.Handler that exposes all the server endpoints
func (h *JobHTTPServer) Handler() http.Handler {
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
	handler = autohead(handler)
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

	n, err := ParseByteUnits(size)
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

	//danger zone
	if r.URL.Query().Get("chunk_size") != "" {
		cs, err := ParseByteUnits(r.URL.Query().Get("chunk_size"))
		chunkSize = int64(cs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if chunkSize == 0 {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// todo- for now we limit chunkSize to maxBodySize. do we a new cmd line parameter for max chunk size ?
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
		Done:    false,
		Expired: false,
		Target:  fmt.Sprintf("download %s to %s (chunk=%d)", size, r.RemoteAddr, chunkSize),
		Job:     h,
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
		Done:    false,
		Expired: false,
		Target:  fmt.Sprintf("upload from %s", r.RemoteAddr),
		Job:     h,
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
	s.StreamPathRegexp = regexp.MustCompile("^(" + ByteUnitsRegexp + ")(\\.[a-zA-Z0-9]+)?$")

	s.Logger = &wm.Logger //

	s.TotalRequests = 0 // just in case we reuse

	host := s.Parameters.Host

	// empty host is a special case

	s.Logger.Debug().Msgf("host parameter is:'%s'", host)
	if host != "" {
		// if host is an interface name
		hostIPAddrs, err := ResolveInterfaceHostAddress(s.Parameters.Host)
		if err != nil {
			return err
		}

		fhostIPAddrs := FilterAddresses(hostIPAddrs, s.Parameters.ForceIPVersion)

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
		Handler:      s.Handler(),
		WriteTimeout: s.Parameters.MaxDuration,
		ReadTimeout:  s.Parameters.MaxDuration,
	}
	if s.Parameters.H2C {
		s.Server.Handler = h2c.NewHandler(s.Handler(), &http2.Server{})
	}

	if s.Parameters.API {
		s.api = &APIHost{Parameters: s.Parameters, Server: s}
		return s.api.Init(wm)
	}
	return nil
}
func (s *JobHTTPServer) Run(ctx context.Context) error {

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

	go func() {
		//select {
		// case sig := <-shutdownCh:
		// 	s.Logger.Warn().Msgf("shutdown started by signal: %s", sig)
		// 	s.Cancel()
		//case

		<-ctx.Done()
		s.Logger.Debug().Msgf("shutdown started by context : %s", ctx.Err())

		shutdownTimeout := s.Parameters.MaxDuration + s.Parameters.MaxDuration*time.Second //todo check with global timeout too
		lctx, lcancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer lcancel()

		s.Server.SetKeepAlivesEnabled(false)
		if err := s.Server.Shutdown(lctx); err != nil {
			s.Logger.Debug().Msgf("shutdown error: %s", err)
		}
		close(exitCh)
	}()

	// TLS stuff
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

	// "tcp4" for IPv4 only
	// "tcp6" or IPv6 only
	// "tcp" (default) for both
	networkTcp := "tcp"
	networkUdp := "udp"
	if s.Parameters.ForceIPVersion == 4 {
		networkTcp = "tcp4"
		networkUdp = "udp4"
	}
	if s.Parameters.ForceIPVersion == 6 {
		networkTcp = "tcp6"
		networkUdp = "udp6"
	}

	var listenErr error
	if s.Parameters.ServeTLS {
		if s.Parameters.HTTP3 {
			// setup debug logger
			quicConf := &quic.Config{}
			if s.Logger.GetLevel() <= zerolog.DebugLevel {
				quicConf.Tracer = qlog.NewTracer(func(_ logging.Perspective, connID []byte) io.WriteCloser {
					filename := fmt.Sprintf("server_%x.qlog", connID)
					f, err := os.Create(filename)
					if err != nil {
						s.Logger.Fatal().Err(err).Send()
					}
					log.Printf("Creating qlog file %s.\n", filename)
					return utils.NewBufferedWriteCloser(bufio.NewWriter(f), f)
				})
			}
			s.Logger.Warn().Msgf("nspeed listening on https://%s (H2C = %t) (http3 on udp)", s.Server.Addr, s.Parameters.H2C)
			listenErr = HTTP3ListenAndServe(s.Server.Addr, networkTcp, networkUdp, s.Server, quicConf)
		} else {
			s.Logger.Warn().Msgf("nspeed listening on https://%s (H2C = %t)", s.Server.Addr, s.Parameters.H2C)
			listenErr = s.Server.ListenAndServeTLS("", "")
		}
	} else {
		s.Logger.Warn().Msgf("nspeed listening on http://%s", s.Server.Addr)
		listenErr = s.Server.ListenAndServe()
	}
	if listenErr != nil && listenErr != http.ErrServerClosed {
		s.Logger.Fatal().Msgf("failed to listen: %s", listenErr)
	}

	<-exitCh
	s.Logger.Info().Msgf("shutdown finished")
	return nil
}

// ListenAndServe listens on the given network addresses for both, TLS and QUIC
// connections in parallel. It returns if one of the two returns an error.
// http.DefaultServeMux is used when handler is nil.
// The correct Alt-Svc headers for QUIC are set.
func HTTP3ListenAndServe(addr, tcpnet, udpnet string, httpServer *http.Server, quicConf *quic.Config) error {

	if httpServer == nil {
		return errors.New("no server")
	}
	// Open the listeners
	udpAddr, err := net.ResolveUDPAddr(udpnet, addr)
	if err != nil {
		return err
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	defer udpConn.Close()

	tcpAddr, err := net.ResolveTCPAddr(tcpnet, addr)
	if err != nil {
		return err
	}
	tcpConn, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return err
	}
	defer tcpConn.Close()

	tlsConn := tls.NewListener(tcpConn, httpServer.TLSConfig)
	defer tlsConn.Close()

	quicServer := &http3.Server{
		Server:     httpServer,
		QuicConfig: quicConf,
	}

	h := httpServer.Handler
	httpServer.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		quicServer.SetQuicHeaders(w.Header())
		h.ServeHTTP(w, r)
	})

	hErr := make(chan error)
	qErr := make(chan error)
	go func() {
		hErr <- httpServer.Serve(tlsConn)
	}()
	go func() {
		qErr <- quicServer.Serve(udpConn)
	}()

	select {
	case err := <-hErr:
		quicServer.Close()
		return err
	case err := <-qErr:
		// Cannot close the HTTP server or wait for requests to complete properly :/
		return err
	}
}
