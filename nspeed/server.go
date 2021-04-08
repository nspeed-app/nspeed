package nspeed

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Default configuration values
const (
	DefaultMaxBodySize int64 = 1024 * 1024 * 1024 * 1024 // bytes
	DefaultMaxDuration       = 10 * time.Second
	DefaultPort              = 7333
)

// parameters of aa server instance
type NSpeedHost struct {
	Host        string
	Port        int
	MaxBodySize int64
	MaxDuration time.Duration
	ServeTLS    bool
	TLSCertFile string
	TLSKeyFile  string
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
	return "nyi"
}

func (j *NSpeedHTTPServer) Expand(jobs []Job) []Job {
	return append(jobs, j)
}

func (j *NSpeedHTTPServer) GoString() string {
	return fmt.Sprintf("server {host: %q, port: %d, max_size: %s, max_duration: %s, UseTLS: %t}",
		j.Parameters.Host,
		j.Parameters.Port,
		ByteCountDecimal(j.Parameters.MaxBodySize, "B"),
		j.Parameters.MaxDuration.String(),
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
	mux.HandleFunc("/p/", h.Upload)

	// return client informations
	mux.HandleFunc("/ip", notImplemented)

	// prevent appending '/'
	mux.HandleFunc("/g", http.NotFound)
	mux.HandleFunc("/p", http.NotFound)

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

	var chunkSize int64 = numBytes

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
		if chunkSize > h.Parameters.MaxBodySize {
			//fmt.Println("chunk is too big {}", chunkSize)
			chunkSize = h.Parameters.MaxBodySize
		}
	} else {
		chunkSize = 10 * 1024 // todo: move this in a parameter/const
	}

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

	src := rand.NewSource(seed)
	rng := rand.New(src)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(numBytes, 10))

	w.WriteHeader(http.StatusOK)

	// prepare a buffer of chunkSize bytes (todo: move this out of here)
	var i int64
	var chunk []byte
	for i = 0; i < chunkSize; i++ {
		chunk = append(chunk, byte(rng.Intn(256)))
	}

	f := w.(http.Flusher)
	// send the buffer as many times as needed
	var numChunk = numBytes / chunkSize
	// fmt.Println("numChunk :", numChunk)
	for i = 0; i < numChunk; i++ {
		w.Write(chunk)
		f.Flush()
	}

	// send the modulo is any
	// fmt.Println("modulo :", numBytes % chunkSize)
	if numBytes%chunkSize > 0 {
		w.Write(chunk[:numBytes%chunkSize])
		f.Flush()
	}
	chunk = nil
}

// handle upload
func (s *NSpeedHTTPServer) Upload(w http.ResponseWriter, r *http.Request) {
	wm := WorkerMetrics{
		Done:    false,
		Expired: false,
		Target:  "upload from %todo",
		Job:     s,
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

	// flag.StringVar(&httpsCertFile, "https-cert-file", "", "HTTPS Server certificate file")
	// flag.StringVar(&httpsKeyFile, "https-key-file", "", "HTTPS Server private key file")

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

	listenAddr := net.JoinHostPort(s.Parameters.Host, strconv.Itoa(s.Parameters.Port))

	server := &http.Server{
		Addr:         listenAddr,
		Handler:      s.Handler(),
		WriteTimeout: s.Parameters.MaxDuration,
		ReadTimeout:  s.Parameters.MaxDuration,
	}

	// shutdownCh triggers graceful shutdown on SIGINT or SIGTERM
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	// exitCh will be closed when it is safe to exit, after graceful shutdown
	exitCh := make(chan struct{})

	go func() {
		sig := <-shutdownCh
		serverLog("shutdown started by signal: %s", sig)

		shutdownTimeout := s.Parameters.MaxDuration + 1*time.Second
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		if err := server.Shutdown(ctx); err != nil {
			serverLog("shutdown error: %s", err)
		}
		close(exitCh)
	}()

	var listenErr error
	if s.Parameters.ServeTLS {
		serverLog("nspeed listening on https://%s", listenAddr)
		listenErr = server.ListenAndServeTLS(s.Parameters.TLSCertFile, s.Parameters.TLSKeyFile)
	} else {
		serverLog("nspeed listening on http://%s", listenAddr)
		listenErr = server.ListenAndServe()
	}
	if listenErr != nil && listenErr != http.ErrServerClosed {
		logger.Fatalf("failed to listen: %s", listenErr)
	}

	<-exitCh
	serverLog("shutdown finished")
	return nil
}
