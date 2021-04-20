package nspeed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var ServerVersion string
var ServerName string

// Default configuration values
const (
	DefaultMaxBodySize int64 = 1000 * 1000 * 1000 * 40 // bytes
	DefaultMaxDuration       = 10 * time.Second
	DefaultPort              = 7333
)

// parameters of aa server instance
type NSpeedHost struct {
	Host           string
	Port           int
	ForceIPVersion int // 0 (none), 4 (force IPv4)  or 6 (force IPv6)
	MaxBodySize    int64
	MaxDuration    time.Duration
	ServeTLS       bool
	TLSCertFile    string
	TLSKeyFile     string
	RunDuration    time.Duration
	MaxRequests    int
}

// Result used for logging
type Result struct {
	Status   int
	Method   string
	URI      string
	Size     int64
	Duration time.Duration
}

// the HTTP Server
// implements Job
type NSpeedHTTPServer struct {
	Parameters *NSpeedHost
	Logger     func(Result)
}

func (j *NSpeedHTTPServer) GetTarget() string {
	return fmt.Sprintf("server %s:%d (%d)", j.Parameters.Host, j.Parameters.Port, j.Parameters.ForceIPVersion)
}

func (j *NSpeedHTTPServer) Expand(jobs []Job) []Job {
	return append(jobs, j)
}

func (j *NSpeedHTTPServer) GoString() string {
	return fmt.Sprintf("server {Address: %q, port: %d, IPversion: %d, max_size: %s, max_duration: %d, UseTLS: %t}",
		j.Parameters.Host,
		j.Parameters.Port,
		j.Parameters.ForceIPVersion,
		ByteCountDecimal(j.Parameters.MaxBodySize)+"B",
		j.Parameters.MaxDuration,
		j.Parameters.ServeTLS,
	)
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

	var chunkSize int64 = 10 * 1024 // todo: move this in a parameter/const

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

	chunk := InitChunk(chunkSize, seed)

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
	wm.ContentSize = numBytes
	f := w.(http.Flusher)
	// send the buffer as many times as needed
	var numChunk = numBytes / chunkSize
	wm.StepSize = chunkSize
	for i := int64(0); i < numChunk; i++ {
		n, err := w.Write(chunk)
		f.Flush()
		wm.TotalRead += int64(n)
		wm.ElapsedTime = time.Since(wm.TransfertStartTime)
		if err != nil {
			wm.Error = err
			wm.Expired = true
			break
		}
	}

	// send the modulo is any
	// fmt.Println("modulo :", numBytes % chunkSize)
	if numBytes%chunkSize > 0 && !wm.Expired {
		n, err := w.Write(chunk[:numBytes%chunkSize])
		f.Flush()
		wm.TotalRead += int64(n)
		wm.ElapsedTime = time.Since(wm.TransfertStartTime)
		if err != nil {
			wm.Error = err
			wm.Expired = true
		}
	}
	wm.Done = true
	wm.Valid = true
	wm.PrintFinal()
}

// handle upload
func (h *NSpeedHTTPServer) Upload(w http.ResponseWriter, r *http.Request) {

	wm := WorkerMetrics{
		Done:    false,
		Expired: false,
		Target:  fmt.Sprintf("upload to %s", r.RemoteAddr),
		Job:     h,
	}
	wm.FirstResponseByte = time.Now()
	n, err := io.Copy(io.Discard, io.TeeReader(r.Body, &wm))
	wm.ElapsedTime = time.Since(wm.FirstResponseByte)
	wm.TotalProcessed = n
	wm.Error = err
	wm.Valid = true
	wm.PrintFinal()
}

// implements the http server
func (s *NSpeedHTTPServer) Start(ctx context.Context, wm *WorkerMetrics) error {

	logger := log.New(os.Stderr, "", 0)

	// A hacky log helper function to ensure that shutdown messages are
	// formatted the same as other messages.
	serverLog := func(msg string, args ...interface{}) {
		const (
			logFmt  = "time=%q msg=%q"
			dateFmt = "2006-01-02T15:04:05.9999"
		)
		logger.Printf(logFmt, time.Now().Format(dateFmt), fmt.Sprintf(msg, args...))
	}
	s.Logger = StdLogFunc(logger)

	host := s.Parameters.Host

	// empty host is a special case

	serverLog("host parameter is:'%s'", host)
	if host != "" {
		// if host is an interface name
		hostIPAddrs, err := ResolveInterfaceHostAddress(s.Parameters.Host)
		if err != nil {
			return err
		}

		fhostIPAddrs := FilterAddresses(hostIPAddrs, s.Parameters.ForceIPVersion)

		if len(fhostIPAddrs) == 0 {
			serverLog("ResolveInterfaceHostAddress/FilterAddresses : %v/%v,%d", hostIPAddrs, fhostIPAddrs, s.Parameters.ForceIPVersion)
			return errors.New("no valid candidate address found")
		}
		host = fhostIPAddrs[0].String()
	}

	// since Addr of http.Server is a single value we only use the first address
	listenAddr := net.JoinHostPort(host, strconv.Itoa(s.Parameters.Port))
	server := &http.Server{
		Addr:         listenAddr,
		Handler:      s.Handler(),
		WriteTimeout: s.Parameters.MaxDuration,
		ReadTimeout:  s.Parameters.MaxDuration,
	}

	// shutdownCh triggers graceful shutdown on SIGINT or SIGTERM
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	cancel := func() {}
	if s.Parameters.RunDuration != 0 {
		newctx, c := context.WithTimeout(ctx, s.Parameters.RunDuration)
		serverLog("server will shutdown in: %v", s.Parameters.RunDuration)
		ctx = newctx
		cancel = c
		defer cancel()
	}
	// exitCh will be closed when it is safe to exit, after graceful shutdown
	exitCh := make(chan struct{})

	go func() {
		select {
		case sig := <-shutdownCh:
			serverLog("shutdown started by signal: %s", sig)
			cancel()
		case <-ctx.Done():
			serverLog("shutdown started by context cancel: %s", ctx.Err())
		}

		shutdownTimeout := s.Parameters.MaxDuration + 1*time.Second
		ctx, cancel := context.WithTimeout(ctx, shutdownTimeout)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		if err := server.Shutdown(ctx); err != nil {
			serverLog("shutdown error: %s", err)
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
		serverLog("nspeed listening on https://%s", listenAddr)
		listenErr = server.ServeTLS(ln, s.Parameters.TLSCertFile, s.Parameters.TLSKeyFile)
	} else {
		serverLog("nspeed listening on http://%s", listenAddr)
		listenErr = server.Serve(ln)
	}
	if listenErr != nil && listenErr != http.ErrServerClosed {
		logger.Fatalf("failed to listen: %s", listenErr)
	}

	<-exitCh
	serverLog("shutdown finished")
	return nil
}
