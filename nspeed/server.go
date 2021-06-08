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
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

var ServerVersion string
var ServerName string
var TLSSelfConfig *tls.Config

// Default configuration values
const (
	DefaultMaxBodySize int64 = 1000 * 1000 * 1000 * 40 // bytes
	DefaultMaxDuration       = 10 * time.Second
	DefaultPort              = 7333
	MaxChunkSize             = 1024 * 1024 // 1 MiB //this will be allocated
	DirPath                  = "/dir/"
)

// parameters of aa server instance
type NSpeedHost struct {
	Host             string
	Port             int
	ForceIPVersion   int // 0 (none), 4 (force IPv4)  or 6 (force IPv6)
	MaxBodySize      int64
	MaxDuration      time.Duration
	ServeTLS         bool
	H2C              bool
	UseTLSSelfConfig bool // if true ignore TLSCertFile/TLSKeyFile use TLSSelfConfig
	TLSCertFile      string
	TLSKeyFile       string
	RunDuration      time.Duration
	MaxRequests      int
	Directory        string // directory to serve content from /dir url
}

// the HTTP Server
// implements Job interface
type NSpeedHTTPServer struct {
	Parameters       *NSpeedHost
	Server           *http.Server
	Cancel           context.CancelFunc
	Logger           *zerolog.Logger
	StreamPathRegexp *regexp.Regexp
	TotalRequests    int
	mutex            sync.Mutex
}

func (j *NSpeedHTTPServer) GetTarget() string {
	return fmt.Sprintf("server %s:%d (IP version: %d)", j.Parameters.Host, j.Parameters.Port, j.Parameters.ForceIPVersion)
}

func (j *NSpeedHTTPServer) Expand(jobs []Job) []Job {
	return append(jobs, j)
}

func (j *NSpeedHTTPServer) GoString() string {
	return fmt.Sprintf("server {Address: %q, port: %d, IPversion: %d, max_size: %s, max_request, max duration: %v, UseTLS: %t, H2C: %t, maxruns: %d, max life: %v}",
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

func (j *NSpeedHTTPServer) PrintString() string {
	return "SERVER"
}

func (j *NSpeedHTTPServer) IsInfinite() bool {
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
func (j *NSpeedHTTPServer) OnRequestStart() bool {
	j.mutex.Lock()
	j.TotalRequests++
	done := j.Parameters.MaxRequests > 0 && j.TotalRequests > j.Parameters.MaxRequests
	defer j.mutex.Unlock()

	if done {
		j.Logger.Info().Msgf("%d max total requests reached, shutting down server", j.TotalRequests)
		return false
	}
	return true
}

// called each time a request is completed, see muxer
func (j *NSpeedHTTPServer) OnRequestEnd() {
	j.mutex.Lock()
	done := j.Parameters.MaxRequests > 0 && j.TotalRequests >= j.Parameters.MaxRequests
	j.mutex.Unlock()
	if done {
		j.Logger.Info().Msgf("%d max total requests reached, shutting down server", j.TotalRequests)
		j.Cancel()
	}
}

func notImplemented(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Handler returns an http.Handler that exposes all the server endpoints
func (h *NSpeedHTTPServer) Handler() http.Handler {
	mux := http.NewServeMux()

	// return client informations
	mux.HandleFunc("/ip", notImplemented)

	// serve static files h.Parameters.Directory
	if h.Parameters.Directory != "" {
		log.Info().Msgf("serving directory '%s' at %s", h.Parameters.Directory, DirPath)
		mux.Handle(DirPath, http.StripPrefix(DirPath, http.FileServer(http.Dir(h.Parameters.Directory))))
	}

	mux.HandleFunc("/", h.rootHandler)

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

// handle '/' .. get : stream bytes, put - upload bytes
func (h *NSpeedHTTPServer) rootHandler(w http.ResponseWriter, r *http.Request) {
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
		}
	}

	if method == "POST" && r.URL.Path == "/" {
		h.Upload(w, r)
		return
	}

	// http.NotFound(w, r)
}

// mostly from https://github.com/mccutchen/go-httpbin
// handleBytes consolidates the logic for validating input params of the Bytes
// and StreamBytes endpoints and knows how to write the response in chunks if
// streaming is true.
func (h *NSpeedHTTPServer) StreamBytes(w http.ResponseWriter, r *http.Request, size string, extension string) {

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
		http.Error(w, "too big", http.StatusBadRequest)
		return
	}

	var chunkSize int64 = 32 * 1024 // todo: move this in a parameter/const, have the client send it eventually or be adaptative?

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
	wm := WorkerMetrics{
		Done:    false,
		Expired: false,
		Target:  fmt.Sprintf("download %s to %s (chunk=%d)", size, r.RemoteAddr, chunkSize),
		Job:     h,
	}
	wm.TransfertStartTime = time.Now()
	wm.FirstResponseByte = time.Now()
	wm.ContentWriteSize = numBytes
	f := w.(http.Flusher)
	// send the buffer as many times as needed
	var numChunk = numBytes / chunkSize
	wm.StepSize = chunkSize
	for i := int64(0); i < numChunk; i++ {
		n, err := w.Write(chunk)
		//f.Flush() -todo investigate
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
		f.Flush()
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
func (h *NSpeedHTTPServer) Upload(w http.ResponseWriter, r *http.Request) {
	h.Logger.Info().Msgf("starting Upload from %s (%d)", r.RemoteAddr, r.ContentLength)
	wm := WorkerMetrics{
		Done:    false,
		Expired: false,
		Target:  fmt.Sprintf("upload from %s", r.RemoteAddr),
		Job:     h,
	}
	wm.FirstResponseByte = time.Now()
	n, err := io.Copy(io.Discard, io.TeeReader(r.Body, &wm))
	wm.ElapsedTime = time.Since(wm.FirstResponseByte)
	wm.TotalReadProcessed = n
	wm.Error = err
	wm.Valid = true
	wm.LogFinal(h.Logger)
}

// implements the http server

func (s *NSpeedHTTPServer) Init(wm *WorkerMetrics) error {

	s.StreamPathRegexp = regexp.MustCompile("^(" + ByteUnitsRegexp + ")(\\.[a-zA-Z0-9]+)?$")

	s.Logger = &wm.Logger //

	s.TotalRequests = 0 // just in case we reuse

	host := s.Parameters.Host

	InitBigChunk(time.Now().Unix())

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
	return nil
}
func (s *NSpeedHTTPServer) Run(ctx context.Context) error {

	// shutdownCh triggers graceful shutdown on SIGINT or SIGTERM
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	// s.Cancel will allows to shutdown the server
	ctx, s.Cancel = context.WithCancel(ctx)
	// run duration is either global timeout from ctx or RunDuration parameter
	if s.Parameters.RunDuration != 0 {
		ctx, s.Cancel = context.WithTimeout(ctx, s.Parameters.RunDuration)
		s.Logger.Debug().Msgf("server will shutdown in: %v", s.Parameters.RunDuration)
		defer s.Cancel()
	}
	// exitCh will be closed when it is safe to exit, after graceful shutdown
	exitCh := make(chan struct{})

	go func() {
		select {
		case sig := <-shutdownCh:
			s.Logger.Warn().Msgf("shutdown started by signal: %s", sig)
			s.Cancel()
		case <-ctx.Done():
			s.Logger.Debug().Msgf("shutdown started by context cancel: %s", ctx.Err())
		}

		shutdownTimeout := s.Parameters.MaxDuration + s.Parameters.MaxDuration*time.Second //todo check with global timeout too
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		s.Server.SetKeepAlivesEnabled(false)
		if err := s.Server.Shutdown(ctx); err != nil {
			s.Logger.Debug().Msgf("shutdown error: %s", err)
		}
		close(exitCh)
	}()

	// "tcp4" for IPv4 only
	// "tcp6" or IPv6 only
	// "tcp" (default) for both
	network := "tcp"
	if s.Parameters.ForceIPVersion == 4 {
		network = "tcp4"
	}
	if s.Parameters.ForceIPVersion == 6 {
		network = "tcp6"
	}

	ln, err := net.Listen(network, s.Server.Addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	var listenErr error
	if s.Parameters.ServeTLS {
		s.Logger.Warn().Msgf("nspeed listening on https://%s (H2C = %t)", s.Server.Addr, s.Parameters.H2C)
		if s.Parameters.UseTLSSelfConfig {
			if TLSSelfConfig != nil {
				s.Server.TLSConfig = TLSSelfConfig
				listenErr = s.Server.ServeTLS(ln, "", "")
			} else {
				// this shouldn't happen if parameters check are done correctly
				s.Logger.Fatal().Msgf("no self certificate installed", listenErr)
			}

		} else {
			listenErr = s.Server.ServeTLS(ln, s.Parameters.TLSCertFile, s.Parameters.TLSKeyFile)
		}
	} else {
		s.Logger.Warn().Msgf("nspeed listening on http://%s", s.Server.Addr)
		listenErr = s.Server.Serve(ln)
	}
	if listenErr != nil && listenErr != http.ErrServerClosed {
		s.Logger.Fatal().Msgf("failed to listen: %s", listenErr)
	}

	<-exitCh
	s.Logger.Info().Msgf("shutdown finished")
	return nil
}
