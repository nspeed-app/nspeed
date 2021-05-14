package nspeed

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"
)

var ServerVersion string
var ServerName string
var TLSSelfConfig *tls.Config

// Default configuration values
const (
	DefaultMaxBodySize int64 = 1000 * 1000 * 1000 * 40 // bytes
	DefaultMaxDuration       = 10 * time.Second
	DefaultPort              = 7333
)

// parameters of aa server instance
type NSpeedHost struct {
	Host             string
	Port             int
	ForceIPVersion   int // 0 (none), 4 (force IPv4)  or 6 (force IPv6)
	MaxBodySize      int64
	MaxDuration      time.Duration
	ServeTLS         bool
	UseTLSSelfConfig bool // if true ignore TLSCertFile/TLSKeyFile use TLSSelfConfig
	TLSCertFile      string
	TLSKeyFile       string
	RunDuration      time.Duration
	MaxRequests      int
}

// the HTTP Server
// implements Job interface
type NSpeedHTTPServer struct {
	Parameters    *NSpeedHost
	Server        *http.Server
	Cancel        context.CancelFunc
	Logger        *zerolog.Logger
	TotalRequests int
	mutex         sync.Mutex
}

func (j *NSpeedHTTPServer) GetTarget() string {
	return fmt.Sprintf("server %s:%d (IP version: %d)", j.Parameters.Host, j.Parameters.Port, j.Parameters.ForceIPVersion)
}

func (j *NSpeedHTTPServer) Expand(jobs []Job) []Job {
	return append(jobs, j)
}

func (j *NSpeedHTTPServer) GoString() string {
	return fmt.Sprintf("server {Address: %q, port: %d, IPversion: %d, max_size: %s, max_request, max duration: %v, UseTLS: %t, maxruns: %d, max life: %v}",
		j.Parameters.Host,
		j.Parameters.Port,
		j.Parameters.ForceIPVersion,
		ByteCountDecimal(j.Parameters.MaxBodySize)+"B",
		j.Parameters.MaxDuration,
		j.Parameters.ServeTLS,
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

	// mux.HandleFunc("/", serverfile)
	mux.HandleFunc("/g/", h.StreamBytes)
	mux.HandleFunc("/p", h.Upload)
	mux.HandleFunc("/p/", h.Upload)

	// return client informations
	mux.HandleFunc("/ip", notImplemented)

	// prevent appending '/'
	mux.HandleFunc("/g", http.NotFound)

	//
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

// mostly from https://github.com/mccutchen/go-httpbin
// handleBytes consolidates the logic for validating input params of the Bytes
// and StreamBytes endpoints and knows how to write the response in chunks if
// streaming is true.
func (h *NSpeedHTTPServer) StreamBytes(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 3 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	n, err := ParseByteUnits(parts[2])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	numBytes := int64(n)

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
		if chunkSize > h.Parameters.MaxBodySize {
			http.Error(w, "chunk size too big", http.StatusBadRequest)
		}
	}
	// end danger zone

	var seed int64
	rawSeed := r.URL.Query().Get("seed")
	if rawSeed != "" {
		seed, err = strconv.ParseInt(rawSeed, 10, 64)
		if err != nil {
			http.Error(w, "invalid seed", http.StatusBadRequest)
			return
		}
	} else {
		seed = time.Now().Unix()
	}

	chunk := InitChunk(chunkSize, seed) //todo: make it global to minimum mem usage (currently it's allocated for every request)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(numBytes, 10))
	w.WriteHeader(http.StatusOK) // this could also be done by the first w.Write()

	// prepare metrics handler
	wm := WorkerMetrics{
		Done:    false,
		Expired: false,
		Target:  fmt.Sprintf("download %s to %s", parts[2], r.RemoteAddr),
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
func (s *NSpeedHTTPServer) Start(ctx context.Context, wm *WorkerMetrics) error {

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

	ln, err := net.Listen(network, listenAddr)
	if err != nil {
		return err
	}
	defer ln.Close()

	var listenErr error
	if s.Parameters.ServeTLS {
		s.Logger.Warn().Msgf("nspeed listening on https://%s", listenAddr)
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
		s.Logger.Warn().Msgf("nspeed listening on http://%s", listenAddr)
		listenErr = s.Server.Serve(ln)
	}
	if listenErr != nil && listenErr != http.ErrServerClosed {
		s.Logger.Fatal().Msgf("failed to listen: %s", listenErr)
	}

	<-exitCh
	s.Logger.Info().Msgf("shutdown finished")
	return nil
}
