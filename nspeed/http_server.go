package nspeed

import (
	"context"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/quic-go/qlog"
	"github.com/rs/xid"
	"github.com/rs/zerolog"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"nspeed.app/utils"
)

// name of product, used in user-agent
var ServerName = "NSpeed"

// local certificate - ALWAYS Clone() it
var TLSSelfConfig *tls.Config

// Default configuration values
const (
	DefaultMaxBodySize int64 = 1000 * 1000 * 1000 * 40 // bytes
	DefaultMaxDuration       = 10 * time.Second
	DefaultIdelTimeout       = 10 * time.Second
	DirPath                  = "/dir/"
	GUIDHeader               = "X-Nspeed-Guid"
	TTLGuidTimeout           = 5 * time.Second
	TTLGuidMaxSize           = 10000
)

var StreamPathRegexp *regexp.Regexp
var TTLGUIDCache *utils.TTLCache[string, *Metric]

func init() {
	StreamPathRegexp = regexp.MustCompile("^(" + utils.ByteUnitsRegexp + ")(\\.[a-zA-Z0-9]+)?$")
	TTLGUIDCache = utils.NewTTLCache[string, *Metric](20, TTLGuidMaxSize)
}

// parameters of a server endpoint
// todo: rename & refactor ?
type HostParameters struct {
	Host             string
	Port             int
	ForceIPVersion   int   // 0 (none), 4 (force IPv4)  or 6 (force IPv6)
	MaxBodySize      int64 // max inbound size and outbound size for streaming (not fileserve)- todo: split into 2 distinct settings?
	MaxDuration      time.Duration
	IdleTimeout      time.Duration
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
}

// initialize a new HostParameters
func NewHostParameters() *HostParameters {
	return &HostParameters{
		//Port:        DefaultPort,
		MaxBodySize: DefaultMaxBodySize,
		MaxDuration: DefaultMaxDuration,
		IdleTimeout: DefaultIdelTimeout,
	}
}

// the HTTP Server
// implements Job interface
type JobHTTPServer struct {
	JobData
	Parameters    *HostParameters
	Server        *http.Server
	Mux           *http.ServeMux
	Cancel        context.CancelFunc
	Close         func(t time.Duration)
	Logger        *zerolog.Logger
	TotalRequests int
	mutex         sync.Mutex
	api           *APIHost
	addr          string
	URL           *url.URL
	listener      *net.Listener
	udpConn       *net.UDPConn
	// allow extension
	InitHook    func() error
	RunHook     func(context.Context) error
	JobList     *JobList
	wm          *Metric
	ResultCache map[string]*Metric
}

func (j *JobHTTPServer) GetTarget() string {
	return fmt.Sprintf("server %s:%d (IP version: %d)", j.Parameters.Host, j.Parameters.Port, j.Parameters.ForceIPVersion)
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

func (j *JobHTTPServer) PrintString(_, _ bool) string {
	return "HTTP SERVER " + j.ID()
}

func (j *JobHTTPServer) GenerateMetric(wm *Metric) (NetMetric, error) {
	return wm.ToNetMetric("unknown origin"), nil
}

func (j *JobHTTPServer) IsInfinite() bool {
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

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// CreateHandler returns an http.Handler that exposes all the server endpoints
func (j *JobHTTPServer) CreateHandler() http.Handler {
	mux := http.NewServeMux()
	j.Mux = mux

	// serve static files h.Parameters.Directory
	if j.Parameters.Directory != "" {
		j.Logger.Info().Msgf("serving directory '%s' at %s", j.Parameters.Directory, DirPath)
		mux.Handle(DirPath, http.StripPrefix(DirPath, http.FileServer(http.Dir(j.Parameters.Directory))))
	}

	if !j.Parameters.APIOnly {
		mux.HandleFunc("/", j.rootHandler)
	}

	mux.HandleFunc("/ip", notImplemented)

	// mux and chain of middlewares (see http_middleware.go)
	var handler http.Handler
	handler = mux
	handler = limitRequestSize(j.Parameters.MaxBodySize, handler)
	handler = preflight(handler)
	handler = limitNumRequests(j.OnRequestStart, j.OnRequestEnd, handler)
	if j.Logger != nil {
		handler = logger(j.Logger, handler)
	}
	// investigate using http.TimeoutHandler
	// handler = http.TimeoutHandler(handler, j.Parameters.MaxDuration, "...")
	return handler
}

// handle '/' .. get : stream bytes, post/put - upload bytes
func (j *JobHTTPServer) rootHandler(w http.ResponseWriter, r *http.Request) {
	method := r.Method
	if method == "" {
		method = "GET"
	}

	j.Logger.Info().Msgf("rootHandler: %s - %s", method, r.URL.Path)

	if method == "GET" {
		a, ok := r.Header[GUIDHeader]
		if ok {
			if len(a) == 1 {
				j.SendCachedMetrics(w, r, a[0])
				return
			}
			http.Error(w, "too many x-nspeed-guid header", http.StatusBadRequest)
			return
		}
		match := StreamPathRegexp.FindStringSubmatch(r.URL.Path[1:])
		j.Logger.Debug().Strs("match", match).Send()
		// keep this test in sync with the regexp
		if len(match) == 3 {
			j.StreamBytes(w, r, match[1], match[2])
			return
		}
		http.Error(w, "Not found (no regexp match)", http.StatusNotFound)
		return
	}

	if method == "POST" && r.URL.Path == "/" {
		j.Upload(w, r)
		return
	}
	//todo: may be only accept some specific path.
	if method == "PUT" {
		j.Upload(w, r)
		return
	}
	//http.NotFound(w, r)
}

// StreamBytes sends 'size' bytes using streaming (io level streaming not HTTP level).
func (j *JobHTTPServer) StreamBytes(w http.ResponseWriter, r *http.Request, size string, extension string) {

	n, err := utils.ParseByteUnits(size)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	numBytes := int64(n)
	j.Logger.Info().Msgf("streaming %s bytes (= %d bytes) with extention: %s", size, numBytes, extension)

	if numBytes < 1 {
		numBytes = 1
	} else if numBytes > j.Parameters.MaxBodySize {
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
		j.Logger.Info().Msgf("chunk_size set to %d", chunkSize)
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

	// x-request-guid
	ip, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		http.Error(w, fmt.Sprintf("cant parse your ip:port (%s)", err), http.StatusBadRequest)
		return
	}
	remoteHash := md5.Sum([]byte(ip.Addr().String()))
	guid := xid.New().String() + "-" + hex.EncodeToString(remoteHash[:])
	w.Header().Set(GUIDHeader, guid)

	// status
	w.WriteHeader(http.StatusOK) // this could also be done by the first w.Write()

	// prepare metrics handler
	wm := Metric{
		MetricsData: MetricsData{
			Job:              j,
			Done:             false,
			Expired:          false,
			Cancelled:        false,
			Target:           fmt.Sprintf("stream %s bytes to %s (chunk=%d)", size, r.RemoteAddr, chunkSize),
			ContentWriteSize: numBytes,
			WriteStepSize:    chunkSize,
		},
		Protocol:   r.Proto,
		Cipher:     TLSCipherSuite(r.TLS),
		TLSVersion: utils.TLSVersionFriendlyName(TLSVersion(r.TLS)),
		Logger: j.Logger.With().
			Str("remote", fmt.Sprintf("%s from %s", r.Method, r.RemoteAddr)).
			Str("x-request-guid", guid).
			Logger(),
	}

	// handle rate parameter
	done, err := j.ParseRate(w, r, &wm)
	if err != nil {
		wm.Logger.Error().Err(err).Msgf("Upload: bad rate value")
		http.Error(w, "bad rate parameter", http.StatusBadRequest)
		return
	}

	wm.TransfertStartTime = time.Now()
	wm.FirstResponseByte = wm.TransfertStartTime
	wm.ContentWriteSize = numBytes //todo: investigate what happens when timed out

	method := r.URL.Query().Get("method")

	if method == "old" {
		//f := w.(http.Flusher)
		// send the buffer as many times as needed --> todo: why don't we io.copy to a wm like the client ?
		var numChunk = numBytes / chunkSize
		wm.WriteStepSize = chunkSize
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
	} else {

		// var src io.Reader = &wm
		// if _, ok := src.(io.WriterTo); ok {
		// 	fmt.Println("WriteTo will be used")
		// }
		// if _, ok := w.(io.ReaderFrom); ok {
		// 	fmt.Println("ReadFrom will be used")
		// }

		// this will disable the ReadFrom of w (http.ResponseWriter)
		// see func (w *response) ReadFrom(src io.Reader) in net/http/server.go
		type writerOnly struct {
			io.Writer
		}
		n, err := io.CopyBuffer(writerOnly{w}, &wm, chunk)
		wm.Error = err
		if n != numBytes {
			wm.Expired = true
		}
	}

	close(done)
	wm.Done = true
	wm.Valid = true
	wm.LogFinal(j.Logger)
	if TTLGUIDCache != nil {
		err := TTLGUIDCache.Set(guid, &wm, TTLGuidTimeout)
		if err != nil {
			http.Error(w, fmt.Sprintf("cant set ttl cache : %s", err), http.StatusBadRequest)
			wm.Logger.Error().Err(err).Msg("cant set ttl cache")
			return
		}
	}
}

func (j *JobHTTPServer) SendCachedMetrics(w http.ResponseWriter, r *http.Request, guid string) {

	j.Logger.Trace().Msgf("getting and deleting guid: %s", guid)
	wm, ok := TTLGUIDCache.Get(guid)
	if !ok {
		http.Error(w, fmt.Sprintf("no such guid (%s)", guid), http.StatusBadRequest)
		return
	}
	err := j.SendMetric(w, r, wm)
	_ = TTLGUIDCache.Delete(guid)
	if err != nil {
		wm.Logger.Error().Err(err).Msgf("Upload: error writing result")
		return
	}
}

func (j *JobHTTPServer) SendMetric(w http.ResponseWriter, r *http.Request, wm *Metric) error {
	m := wm.ToNetMetric(fmt.Sprintf("%s from %s (%d bytes)", r.Method, r.RemoteAddr, r.ContentLength))
	result, err := json.MarshalIndent(m, "", "\t")
	if err != nil {
		_, err = fmt.Fprintf(w, "error formating metrics: %v", err)
		if err != nil {
			wm.Logger.Error().Err(err).Msgf("Upload: error sending reply")
			return err
		}
		return err
	}
	_, err = w.Write(result)
	return err
}

func (j *JobHTTPServer) ParseRate(w http.ResponseWriter, r *http.Request, wm *Metric) (chan bool, error) {
	var err error
	if rate := r.URL.Query().Get("rate"); rate != "" {
		j.Logger.Info().Msgf("Upload: %s from %s - set rate to %s", r.Method, r.RemoteAddr, rate)
		wm.Tick, err = time.ParseDuration(rate)
		if err != nil {
			http.Error(w, fmt.Sprintf("bad value '%s' for rate", rate), http.StatusBadRequest)
			return nil, err
		}
	}
	done := make(chan bool)
	wm.StartTicker(done)
	return done, nil
}

// handle upload
func (j *JobHTTPServer) Upload(w http.ResponseWriter, r *http.Request) {

	j.Logger.Info().Msgf("starting Upload (%s) from %s (%d)", r.Method, r.RemoteAddr, r.ContentLength)

	// metric for this request
	wm := &Metric{
		MetricsData: MetricsData{
			Done:               false,
			Expired:            false,
			Target:             fmt.Sprintf("upload from %s, path = %s", r.RemoteAddr, r.RequestURI),
			Job:                j,
			TransfertStartTime: time.Now(),
			FirstResponseByte:  time.Now(),
		},
		Protocol:   r.Proto,
		Cipher:     TLSCipherSuite(r.TLS),
		TLSVersion: utils.TLSVersionFriendlyName(TLSVersion(r.TLS)),
		Logger:     j.Logger.With().Str("remote", fmt.Sprintf("%s from %s", r.Method, r.RemoteAddr)).Logger(),
	}

	// handle rate parameter
	done, err := j.ParseRate(w, r, wm)
	if err != nil {
		wm.Logger.Error().Err(err).Msgf("Upload: bad rate value")
		return
	}
	// do the upload
	n, err := io.CopyBuffer(io.Discard, io.TeeReader(r.Body, wm), BigBuffer)
	// signal the evenutal ticker
	close(done)

	wm.ElapsedTime = time.Since(wm.FirstResponseByte)
	wm.TotalRead = n
	wm.Error = err
	wm.Valid = true
	wm.LogFinal(j.Logger)

	// send the metrics
	err = j.SendMetric(w, r, wm)

	if err != nil {
		wm.Logger.Error().Err(err).Msgf("Upload: error writing result")
		return
	}
}

// implements the http server

func (j *JobHTTPServer) Init(wm *Metric) error {

	j.wm = wm
	j.Logger = &wm.Logger //

	j.TotalRequests = 0 // just in case we reuse

	host := j.Parameters.Host

	// empty host is a special case

	j.Logger.Debug().Msgf("host parameter is:'%s'", host)
	if host != "" {
		// if host is an interface name
		hostIPAddrs, err := utils.ResolveInterfaceHostAddress(j.Parameters.Host)
		if err != nil {
			return err
		}

		fhostIPAddrs := utils.FilterAddresses(hostIPAddrs, j.Parameters.ForceIPVersion)

		if len(fhostIPAddrs) == 0 {
			j.Logger.Debug().Msgf("ResolveInterfaceHostAddress/FilterAddresses : %v/%v,%d", hostIPAddrs, fhostIPAddrs, j.Parameters.ForceIPVersion)
			return errors.New("no valid candidate address found")
		}
		host = fhostIPAddrs[0].String()
	}

	// since Addr of http.Server is a single value we only use the first address
	listenAddr := net.JoinHostPort(host, strconv.Itoa(j.Parameters.Port))
	j.Server = &http.Server{
		Addr:              listenAddr,
		Handler:           j.CreateHandler(),
		WriteTimeout:      j.Parameters.MaxDuration, // to handle this per request
		ReadTimeout:       j.Parameters.MaxDuration, // idem
		ReadHeaderTimeout: j.Parameters.MaxDuration,
		IdleTimeout:       j.Parameters.IdleTimeout,
	}

	if j.Parameters.H2C {
		j.Server.Handler = h2c.NewHandler(j.Server.Handler, &http2.Server{})
	}

	if j.Parameters.API {
		j.api = &APIHost{Parameters: j.Parameters, Server: j}
		return j.api.Init(wm)
	}
	return nil
}
func (j *JobHTTPServer) Run(ctx context.Context, wg *sync.WaitGroup) error {

	ctx, j.Cancel = context.WithCancel(ctx)

	// run duration is either global timeout from ctx or RunDuration parameter
	if j.Parameters.RunDuration != 0 {
		ctx2, cancel := context.WithTimeout(ctx, j.Parameters.RunDuration)
		defer cancel()
		ctx = ctx2
		j.Logger.Debug().Msgf("server will shutdown in: %v", j.Parameters.RunDuration)
	}
	// exitCh will be closed when it is safe to exit, after graceful shutdown
	exitCh := make(chan struct{})

	// call this to shutdown server (net.http version)
	j.Close = func(shutdownTimeout time.Duration) {
		lctx, lcancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer lcancel()

		j.Server.SetKeepAlivesEnabled(false)
		if err := j.Server.Shutdown(lctx); err != nil {
			j.Logger.Debug().Msgf("shutdown error: %s", err)
		}
	}

	go func() {
		<-ctx.Done()
		j.Logger.Debug().Msgf("shutdown started by context : %s", ctx.Err())

		j.Close(j.Parameters.MaxDuration + 1*time.Second) //todo check with global timeout too

		close(exitCh)
	}()

	// TLS stuff (todo move this in Init ?)
	if j.Parameters.UseTLSSelfConfig {
		if TLSSelfConfig != nil {
			j.Server.TLSConfig = TLSSelfConfig.Clone()
			j.Logger.Info().Msgf("using TLSSelfConfig")
		} else {
			// this shouldn't happen if parameters check are done correctly
			j.Logger.Fatal().Msgf("no self certificate installed")
		}
	} else if j.Parameters.ServeTLS {
		// todo: this should be done earlier may be ?
		cer, err := tls.LoadX509KeyPair(j.Parameters.TLSCertFile, j.Parameters.TLSKeyFile)
		if err != nil {
			j.Logger.Fatal().Err(err).Msgf("error loading cert/key files")
		}
		j.Server.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cer}}
	}

	err := j.Listen(ctx)
	if err != nil {
		j.Logger.Fatal().Msgf("failed to listen: %s", err)
		return err
	}

	// signal parent we're listening
	wg.Done()
	// hack: openbrowser, revisit this
	if j.RunHook != nil {
		err := j.RunHook(ctx)
		if err != nil {
			j.Logger.Warn().Msgf("failed to serve: %s", err)
			return err
		}
	}

	// Serve: todo
	err = j.Serve()
	// remove from global list

	if err != nil {
		if err != errors.New("quic: Server closed") { // temporary quic fix
			j.Logger.Warn().Msgf("failed to serve: %s", err)
		}
	}

	// we get here if shutdown or error
	<-exitCh
	if err == nil {
		j.Logger.Info().Msgf("shutdown finished")
	}

	// signal parent and send metrics
	j.wm.ResultChan <- j.wm
	return nil
}

// Listen opens the listening conn
func (j *JobHTTPServer) Listen(ctx context.Context) error {

	if j.Parameters.HTTP3 {
		return j.HTTP3Listen()
	}

	if j.listener != nil {
		return errors.New("a connection is already listening")
	}

	j.addr = j.Server.Addr
	if j.addr == "" {
		j.addr = ":http"
		if j.Parameters.ServeTLS {
			j.addr = ":https"
		}
	}

	lc := net.ListenConfig{
		Control: func(network, address string, conn syscall.RawConn) error {
			var operr error
			if err := conn.Control(func(fd uintptr) {
				//operr = syscall.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_RCVBUF, 1)
				buf, err := syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF)
				if err != nil {
					return
				}
				j.Logger.Info().Msgf("getsockopt rcvbuf = %d", buf)
				buf, err = syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF)
				if err != nil {
					return
				}
				j.Logger.Info().Msgf("getsockopt sndbuf = %d", buf)

			}); err != nil {
				return err
			}
			return operr
		},
	}

	ln, err := lc.Listen(ctx, "tcp", j.addr)
	if err != nil {
		return err
	}
	j.listener = &ln
	if j.Parameters.ServeTLS {
		j.URL = &url.URL{Scheme: "https", Host: ln.Addr().String()}
		j.Logger.Warn().Msgf("server %s listening on %s (H2C = %t)", j.ID(), j.URL.String(), j.Parameters.H2C)
	} else {
		j.URL = &url.URL{Scheme: "http", Host: ln.Addr().String()}
		j.Logger.Warn().Msgf("server %s listening on %s", j.ID(), j.URL.String())
	}

	return nil
}

// Serve accepts incoming connections for this server.
func (j *JobHTTPServer) Serve() error {
	err := j.serve()
	if err == http.ErrServerClosed {
		err = nil
	}
	return err
}

// serve implements Serve/ServeTLS based on parameters
func (j *JobHTTPServer) serve() error {

	if j.Parameters.HTTP3 {
		return j.HTTP3Serve()
	}

	if j.listener == nil {
		return errors.New("no listener")
	}

	if j.Parameters.HTTP2 {
		return j.HTTP2Serve()
	} else if j.Parameters.ServeTLS {
		if j.Parameters.HTTP11 {
			// force HTTP1.1 as per https://pkg.go.dev/net/http#pkg-overview
			j.Server.TLSNextProto = map[string]func(*http.Server, *tls.Conn, http.Handler){}
		}
		return j.Server.ServeTLS(*j.listener, "", "")

	} else {
		return j.Server.Serve(*j.listener)
	}

}

// nolint
func (j *JobHTTPServer) HTTP2Serve() error {
	//return errors.New("enforcing HTTP/2 is not implemented yet")
	return j.HTTP2serve()
	// can't do unless we rewrite Serve because of we can't add
	// our own proto handler for http/1.1 (see )

	//revive:disable
	fmt.Println("using http/2 only with patched crypto")
	//revive:enable

	err := http2.ConfigureServer(j.Server, &http2.Server{})

	if err != nil {
		return err
	}

	idx := utils.IndexOf(j.Server.TLSConfig.NextProtos, "http/1.1")
	if idx != -1 {
		j.Logger.Warn().Msgf("removing http/1.1 from server.TLSConfig")
		j.Server.TLSConfig.NextProtos = append(j.Server.TLSConfig.NextProtos[:idx], j.Server.TLSConfig.NextProtos[idx+1:]...)
	}

	config := j.Server.TLSConfig.Clone()

	tlsListener := tls.NewListener(*j.listener, config)
	return j.Server.Serve(tlsListener)
}

// HTTP3Listen opens the listening udp conn
func (j *JobHTTPServer) HTTP3Listen() error {

	if j.Server == nil {
		return errors.New("no HTTP server")
	}

	if j.udpConn != nil {
		return errors.New("udp connection already listening")
	}

	networkUDP := "udp"
	if j.Parameters.ForceIPVersion == 4 {
		networkUDP = "udp4"
	}
	if j.Parameters.ForceIPVersion == 6 {
		networkUDP = "udp6"
	}

	// Open the listeners
	udpAddr, err := net.ResolveUDPAddr(networkUDP, j.Server.Addr)
	if err != nil {
		return err
	}
	j.udpConn, err = net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	j.URL = &url.URL{Scheme: "https", Host: j.udpConn.LocalAddr().String()}
	j.Logger.Warn().Msgf("server listening on %s (HTTP/3 on UDP)", j.URL.String())
	return nil
}

func (j *JobHTTPServer) HTTP3Serve() error {

	if j.udpConn == nil {
		return errors.New("no udp connection")
	}
	defer j.udpConn.Close()

	quicConf := &quic.Config{}

	// setup debug trace
	if zerolog.GlobalLevel() <= zerolog.TraceLevel {
		quicConf.Tracer = qlog.NewTracer(utils.QLogTracer(j.Logger))
	}

	quicServer := &http3.Server{
		Addr:       j.Server.Addr,
		TLSConfig:  j.Server.TLSConfig,
		QuicConfig: quicConf,
		Handler:    j.Server.Handler,
	}

	h := j.Server.Handler
	j.Server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := quicServer.SetQuicHeaders(w.Header())
		j.Logger.Debug().Msgf("SetQuicHeaders called")
		if err != nil {
			j.Logger.Fatal().Err(err)
		}
		h.ServeHTTP(w, r)
	})

	// this blocks or fail
	j.Logger.Info().Msgf("nspeed serving https://%s (http3 on udp)", j.Server.Addr)

	j.Close = func(shutdownTimeout time.Duration) {
		j.Logger.Debug().Msgf("warning CloseGracefully() NYI for quic server, shuting down with Close() -> expect a fatal error")
		// nyi: quicServer.CloseGracefully(shutdownTimeout)
		err := quicServer.Close()
		j.Logger.Info().Err(err).Msg("quicServer Close called")
	}
	return quicServer.Serve(j.udpConn)
}
