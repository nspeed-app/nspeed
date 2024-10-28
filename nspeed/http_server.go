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
	"slices"
	"strconv"
	"sync"
	"time"

	quicgo "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/rs/xid"
	"github.com/rs/zerolog"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"nspeed.app/metric"
	nsutl "nspeed.app/nspeed/utils"
	"nspeed.app/utils"
)

// name of product, used in user-agent
var ServerName = "NSpeed"

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
var TTLGUIDCache *utils.TTLCache[string, *metric.Metric]

func init() {
	StreamPathRegexp = regexp.MustCompile("^(" + utils.ByteUnitsRegexp + ")(\\.[a-zA-Z0-9]+)?$")
	TTLGUIDCache = utils.NewTTLCache[string, *metric.Metric](20, TTLGuidMaxSize)
}

// the HTTP Server
// implements Job interface
type JobHTTPServer struct {
	JobData
	Parameters    *HostParameters
	Server        *http.Server
	Mux           *http.ServeMux
	Cancel        context.CancelFunc
	Shutdown      func(t time.Duration)
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
	wm          *metric.Metric
	ResultCache map[string]*metric.Metric
}

func (j *JobHTTPServer) GetURL() *url.URL {
	return j.URL
}

func (j *JobHTTPServer) GetTarget() string {
	return fmt.Sprintf("server %s:%d (IP version: %d)", j.Parameters.Host, j.Parameters.Port, j.Parameters.IPVersion)
}

func (j *JobHTTPServer) GoString() string {
	if zerolog.GlobalLevel() > zerolog.InfoLevel {
		return fmt.Sprintf("HTTP server %s:%d", j.Parameters.Host, j.Parameters.Port)
	}
	return fmt.Sprintf("HTTP server {Address: %q, port: %d, IPversion: %d, max_size: %s, max_request, max duration: %v, UseTLS: %t, H2C: %t, maxruns: %d, max life: %v}",
		j.Parameters.Host,
		j.Parameters.Port,
		j.Parameters.IPVersion,
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
		j.Logger().Info().Msgf("%d max total requests reached", j.TotalRequests)
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
		j.Logger().Info().Msgf("%d max total requests reached", j.TotalRequests)
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
		j.Logger().Info().Msgf("serving directory '%s' at %s", j.Parameters.Directory, DirPath)
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
	if j.Logger() != nil {
		handler = logger(j.Logger(), handler)
	}
	// investigate using http.TimeoutHandler
	// handler = http.TimeoutHandler(handler, j.Parameters.MaxDuration, "...")
	return handler
}

// return informations to the clients
func (j *JobHTTPServer) LandingPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	rr := struct {
		Server  string
		Method  string
		Proto   string
		Remote  string
		Cipher  string
		Headers http.Header
	}{
		Server:  "nspeed " + Version,
		Method:  r.Method,
		Proto:   r.Proto,
		Remote:  r.RemoteAddr,
		Headers: r.Header,
	}
	if r.TLS != nil {
		rr.Cipher = tls.CipherSuiteName(r.TLS.CipherSuite)
	}
	_ = json.NewEncoder(w).Encode(rr)
}

// handle '/' .. get : stream bytes, post/put - upload bytes
func (j *JobHTTPServer) rootHandler(w http.ResponseWriter, r *http.Request) {
	method := r.Method
	if method == "" {
		method = "GET"
	}

	j.Logger().Info().Msgf("rootHandler: %s - %s", method, r.URL.Path)

	if method == "GET" {
		a, ok := r.Header[GUIDHeader]
		if ok {
			if len(a) == 1 {
				j.SendCachedMetrics(w, a[0])
				return
			}
			http.Error(w, "too many x-nspeed-guid header", http.StatusBadRequest)
			return
		}
		if r.URL.Path == "/" {
			j.LandingPage(w, r)
			return
		}
		match := StreamPathRegexp.FindStringSubmatch(r.URL.Path[1:])
		j.Logger().Debug().Strs("match", match).Send()
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
	j.Logger().Info().Msgf("streaming %s bytes (= %d bytes) with extention: %s", size, numBytes, extension)

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
			http.Error(w, "chunk size is zero", http.StatusBadRequest)
			return
		}
		// todo- for now we limit chunkSize to maxBodySize. do we need a new cmd line parameter for max chunk size ?
		// since huge chunksize can overload the server memory this is not safe
		if chunkSize > utils.MaxChunkSize {
			http.Error(w, fmt.Sprintf("chunk size too big (max allowed is %d)", utils.MaxChunkSize), http.StatusBadRequest)
			return
		}
		j.Logger().Info().Msgf("chunk_size set to %d", chunkSize)
	}
	// end danger zone

	chunk := utils.GetChunk(chunkSize)

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
	wm := metric.Metric{
		MetricsData: metric.MetricsData{
			Command:          fmt.Sprintf("%s %s from %s (%d bytes)", r.Method, r.URL.Path, r.RemoteAddr, numBytes),
			Done:             false,
			Expired:          false,
			Cancelled:        false,
			Target:           fmt.Sprintf("stream %s bytes to %s (chunk=%d)", size, r.RemoteAddr, chunkSize),
			ContentWriteSize: numBytes,
			WriteStepSize:    chunkSize,
			ID:               guid,
			GroupID:          guid,
		},
		Protocol:   r.Proto,
		Cipher:     utils.TLSCipherSuite(r.TLS),
		TLSVersion: utils.TLSVersionFriendlyName(utils.TLSVersion(r.TLS)),
	}
	defer wm.Close()

	wm.SetLogger(utils.Ptr(j.Logger().With().
		Str("remote", fmt.Sprintf("%s from %s", r.Method, r.RemoteAddr)).
		Str("x-request-guid", guid).
		Logger()))

	wm.Remote, err = netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		wm.Logger().Debug().Err(err).Msgf("error parsing remote address: %s", r.RemoteAddr)
	}

	// handle rate parameter
	done, err := j.ParseRate(w, r, &wm)
	if err != nil {
		wm.Logger().Error().Err(err).Msgf("Upload: bad rate value")
		http.Error(w, "bad rate parameter", http.StatusBadRequest)
		return
	}

	wm.TransfertStartTime = time.Now()
	wm.FirstResponseByte = wm.TransfertStartTime
	wm.ContentWriteSize = numBytes //todo: investigate what happens when timed out

	method := r.URL.Query().Get("method")

	if method == "old" {
		wm.Logger().Info().Err(err).Msgf("using old method")
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
		rc := http.NewResponseController(w)
		err := rc.SetWriteDeadline(time.Now().Add(j.Parameters.MaxDuration))
		wm.Logger().Info().Msgf("setting write deadline of %s\n", j.Parameters.MaxDuration)
		if err != nil {
			wm.Logger().Info().Err(err).Msgf("cant set deadline")
		}

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
	wm.LogFinal(j.Logger())
	if TTLGUIDCache != nil {
		err := TTLGUIDCache.Set(guid, &wm, TTLGuidTimeout)
		if err != nil {
			http.Error(w, fmt.Sprintf("cant set ttl cache : %s", err), http.StatusBadRequest)
			wm.Logger().Error().Err(err).Msg("cant set ttl cache")
			return
		}
	}
}

func (j *JobHTTPServer) SendCachedMetrics(w http.ResponseWriter, guid string) {

	j.Logger().Trace().Msgf("getting and deleting guid: %s", guid)
	wm, ok := TTLGUIDCache.Get(guid)
	if !ok {
		http.Error(w, fmt.Sprintf("no such guid (%s)", guid), http.StatusBadRequest)
		return
	}
	err := j.SendMetric(w, wm)
	_ = TTLGUIDCache.Delete(guid)
	if err != nil {
		wm.Logger().Error().Err(err).Msgf("Upload: error writing result")
		return
	}
}

func (j *JobHTTPServer) SendMetric(w http.ResponseWriter, wm *metric.Metric) error {
	m := wm.ToNetMetric(nsutl.FormatV4full, nsutl.FormatV6Full)
	result, err := json.MarshalIndent(m, "", "\t")
	if err != nil {
		_, err = fmt.Fprintf(w, "error formating metrics: %v", err)
		if err != nil {
			wm.Logger().Error().Err(err).Msgf("Upload: error sending reply")
			return err
		}
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(result)
	return err
}

const safeRate = 1 * time.Millisecond // TODO: place this else and/or as a global setting

func (j *JobHTTPServer) ParseRate(w http.ResponseWriter, r *http.Request, wm *metric.Metric) (chan any, error) {
	if rate := r.URL.Query().Get("rate"); rate != "" {
		j.Logger().Info().Msgf("Upload: %s from %s - set rate to %s", r.Method, r.RemoteAddr, rate)
		tick, err := time.ParseDuration(rate)
		if err != nil {
			http.Error(w, fmt.Sprintf("bad value '%s' for rate", rate), http.StatusBadRequest)
			return nil, err
		}
		if tick < safeRate {
			http.Error(w, fmt.Sprintf("too small value '%s' for rate", rate), http.StatusBadRequest)
			return nil, err
		}
		wm.Tick = tick
	}
	done := make(chan any)
	wm.StartTicker(done)
	return done, nil
}

// handle upload
func (j *JobHTTPServer) Upload(w http.ResponseWriter, r *http.Request) {

	j.Logger().Info().Msgf("starting Upload (%s) from %s (%d)", r.Method, r.RemoteAddr, r.ContentLength)

	// metric for this request
	wm := &metric.Metric{
		MetricsData: metric.MetricsData{
			Command:            fmt.Sprintf("%s %s from %s (%d)", r.Method, r.URL.Path, r.RemoteAddr, r.ContentLength),
			Done:               false,
			Expired:            false,
			Target:             fmt.Sprintf("upload from %s, path = %s", r.RemoteAddr, r.RequestURI),
			TransfertStartTime: time.Now(),
			FirstResponseByte:  time.Now(),
		},
		Protocol:   r.Proto,
		Cipher:     utils.TLSCipherSuite(r.TLS),
		TLSVersion: utils.TLSVersionFriendlyName(utils.TLSVersion(r.TLS)),
	}
	wm.SetLogger(utils.Ptr(j.Logger().With().Str("remote", fmt.Sprintf("%s from %s", r.Method, r.RemoteAddr)).Logger()))

	// handle rate parameter
	done, err := j.ParseRate(w, r, wm)
	if err != nil {
		wm.Logger().Error().Err(err).Msgf("Upload: bad rate value")
		return
	}

	//
	rc := http.NewResponseController(w)
	err = rc.SetReadDeadline(time.Now().Add(j.Parameters.MaxDuration))
	wm.Logger().Info().Msgf("setting read deadline of %s\n", j.Parameters.MaxDuration)
	if err != nil {
		wm.Logger().Info().Err(err).Msgf("cant set read deadline")
	}

	// do the upload
	//n, err := io.CopyBuffer(io.Discard, io.TeeReader(r.Body, wm), BigBuffer)
	n, err := io.CopyBuffer(wm, r.Body, utils.BigBuffer)
	// signal the evenutal ticker
	close(done)

	wm.ElapsedTime = time.Since(wm.FirstResponseByte)
	wm.TotalRead = n
	wm.Error = err
	wm.Valid = true
	wm.LogFinal(j.Logger())

	// send the metrics
	err = j.SendMetric(w, wm)

	if err != nil {
		wm.Logger().Error().Err(err).Msgf("Upload: error writing result")
		return
	}
}

// implements the http server

func (j *JobHTTPServer) Init(opt Options) error {

	j.wm = &metric.Metric{
		MetricsData: metric.MetricsData{
			Done:    false,
			Expired: false,
			Target:  j.GetTarget(),
			ID:      j.ID(),
			GroupID: j.GroupID(),
		},
		//Tick:       opt.Tick,
	}
	j.wm.SetLogger(j.Logger())

	j.TotalRequests = 0 // just in case we reuse

	host := j.Parameters.Host

	// empty host is a special case

	ipVersion := utils.OrDefault(j.Parameters.IPVersion, opt.IPVersion)

	j.Logger().Debug().Msgf("host parameter is:'%s'", host)
	if host != "" {
		// if host is an interface name
		hostIPAddrs, err := utils.ResolveInterfaceHostAddress(j.Parameters.Host)
		if err != nil {
			return err
		}

		fhostIPAddrs := utils.FilterAddresses(hostIPAddrs, ipVersion)

		if len(fhostIPAddrs) == 0 {
			j.Logger().Debug().Msgf("ResolveInterfaceHostAddress/FilterAddresses : %v/%v,%d", hostIPAddrs, fhostIPAddrs, ipVersion)
			return errors.New("no valid candidate address found")
		}
		host = fhostIPAddrs[0].String()
	}
	// todo: we need to enforce ipversion if host==""

	// since Addr of http.Server is a single value we only use the first address
	listenAddr := net.JoinHostPort(host, strconv.Itoa(j.Parameters.Port))
	j.Server = &http.Server{
		Addr:              listenAddr,
		Handler:           j.CreateHandler(),
		WriteTimeout:      0,               //j.Parameters.MaxDuration, // todo handle this per request
		ReadTimeout:       0,               //j.Parameters.MaxDuration, // idem
		ReadHeaderTimeout: 2 * time.Second, //j.Parameters.MaxDuration,
		IdleTimeout:       j.Parameters.IdleTimeout,
		//ConnState:        todo,
	}
	// TLS settings to tls.Config
	var err error
	j.Server.TLSConfig, err = j.Parameters.MakeTLSConfig()
	if err != nil {
		j.Logger().Error().Err(err).Msgf("MakeTLSConfig error")
		return fmt.Errorf("MakeTLSConfig error: %w", err)
	}

	if j.Parameters.H2C {
		j.Server.Handler = h2c.NewHandler(j.Server.Handler, &http2.Server{})
	}

	if j.Parameters.API {
		j.api = &APIHost{Parameters: j.Parameters, Server: j}
		return j.api.Init(opt)
	}
	return nil
}
func (j *JobHTTPServer) Run(ctx context.Context, wg *sync.WaitGroup, _ Options) (*metric.Metric, error) {

	done := false
	defer func() {
		if !done {
			wg.Done()
			done = true // not used but just in case
		}
	}()

	ctx, j.Cancel = context.WithCancel(ctx)

	// run duration is either global timeout from ctx or RunDuration parameter
	if j.Parameters.RunDuration != 0 {
		ctx2, cancel := context.WithTimeout(ctx, j.Parameters.RunDuration)
		defer cancel()
		ctx = ctx2
		j.Logger().Debug().Msgf("server will shutdown in: %v", j.Parameters.RunDuration)
	}
	// exitCh will be closed when it is safe to exit, after graceful shutdown
	exitCh := make(chan struct{})

	// call this to shutdown server (net.http version)
	j.Shutdown = func(shutdownTimeout time.Duration) {
		lctx, lcancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer lcancel()

		j.Server.SetKeepAlivesEnabled(false)
		if err := j.Server.Shutdown(lctx); err != nil {
			j.Logger().Debug().Msgf("shutdown error: %s", err)
		}
	}

	go func() {
		//todo:is ctx is never cancelled, can this mem leak a goroutine for ever
		<-ctx.Done()
		j.Logger().Debug().Msgf("shutdown started by context : %s", ctx.Err())

		j.Shutdown(j.Parameters.MaxDuration + 1*time.Second) //todo check with global timeout too

		close(exitCh)
	}()

	err := j.Listen(ctx)
	if err != nil {
		j.Logger().Error().Err(err).Msgf("failed to listen: %v", err)
		return nil, fmt.Errorf("Listen failed: %w", err)
	}

	// signal parent we're listening
	wg.Done()
	done = true
	// hack: openbrowser, revisit this
	if j.RunHook != nil {
		err := j.RunHook(ctx)
		if err != nil {
			j.Logger().Error().Err(err).Msgf("RunHook failed: %v", err)
			return nil, fmt.Errorf("RunHook failed: %w", err)
		}
	}

	// Serve: todo
	err = j.Serve()
	// remove from global list

	if err != nil {
		if err != errors.New("quic: Server closed") { // temporary quic fix
			j.Logger().Warn().Msgf("failed to serve: %s", err)
		}
	}

	// we get here if shutdown or error
	<-exitCh
	if err == nil {
		j.Logger().Info().Msgf("shutdown finished")
	}

	// send metrics
	return j.wm, err
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
	// default port is 80 for http and 443 for https
	if j.addr == "" {
		j.addr = ":http"
		if j.Parameters.ServeTLS {
			j.addr = ":https"
		}
	}

	lc := net.ListenConfig{
		// Control: func(network, address string, conn syscall.RawConn) error {
		// 	var operr error
		// 	if err := conn.Control(func(fd uintptr) {
		// 		buf, err := webview.GetSockOpt(fd, syscall.SO_RCVBUF)
		// 		if err != nil {
		// 			j.Logger().Trace().Err(err).Msgf("getsockopt rcvbuf")
		// 			return
		// 		}
		// 		j.Logger().Info().Msgf("getsockopt rcvbuf = %d", buf)
		// 		buf, err = webview.GetSockOpt(fd, syscall.SO_SNDBUF)
		// 		if err != nil {
		// 			j.Logger().Trace().Err(err).Msgf("getsockopt sndbuf")
		// 			return
		// 		}
		// 		j.Logger().Trace().Msgf("getsockopt sndbuf = %d", buf)
		// 	}); err != nil {
		// 		return err
		// 	}
		// 	return operr
		// },
	}

	// enforce ipVersion here
	network := utils.AddIPVersionToNetwork("tcp", j.Parameters.IPVersion)

	ln, err := lc.Listen(ctx, network, j.addr)
	if err != nil {
		return err
	}
	j.listener = &ln
	if j.Parameters.ServeTLS {
		j.URL = &url.URL{Scheme: "https", Host: ln.Addr().String()}
		j.Logger().Warn().Msgf("server %s listening on %s (H2C = %t)", j.ID(), j.URL.String(), j.Parameters.H2C)
	} else {
		j.URL = &url.URL{Scheme: "http", Host: ln.Addr().String()}
		j.Logger().Warn().Msgf("server %s listening on %s", j.ID(), j.URL.String())
	}

	return nil
}

// Serve accepts incoming connections for this server.
func (j *JobHTTPServer) Serve() error {
	err := j.serve()
	if err == http.ErrServerClosed { //|| err == quic.ErrServerClosed {
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

	}
	return j.Server.Serve(*j.listener)
}

// nolint
func (j *JobHTTPServer) HTTP2Serve() error {

	err := http2.ConfigureServer(j.Server, &http2.Server{})

	if err != nil {
		return err
	}

	idx := slices.Index(j.Server.TLSConfig.NextProtos, "http/1.1")
	if idx != -1 {
		j.Logger().Warn().Msgf("removing http/1.1 from server.TLSConfig")
		j.Server.TLSConfig.NextProtos = append(j.Server.TLSConfig.NextProtos[:idx], j.Server.TLSConfig.NextProtos[idx+1:]...)
	}

	config := j.Server.TLSConfig.Clone()

	// we could use tls.Config.GetConfigForClient also
	// this will usually drops the client with an ssl error 32 on its side
	config.VerifyConnection = func(cs tls.ConnectionState) error {
		if cs.NegotiatedProtocol != "h2" {
			return fmt.Errorf("tls: client doesn't support h2")
		}
		return nil
	}

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
	var err error
	j.udpConn, err = listenUDP(j.Server.Addr, j.Parameters.IPVersion)

	if err != nil {
		return fmt.Errorf("couldn't udp listen %s (error=%w)", j.Server.Addr, err)
	}

	j.URL = &url.URL{Scheme: "https", Host: j.udpConn.LocalAddr().String()}
	j.Logger().Warn().Msgf("server listening on %s (HTTP/3 on UDP)", j.URL.String())
	return nil
}

// listenUDP performs net.ResolveUDPAddr then net.ListenUDP
func listenUDP(addr string, version utils.IPVersion) (*net.UDPConn, error) {
	networkUDP := utils.AddIPVersionToNetwork("udp", version)

	// Open the listeners
	udpAddr, err := net.ResolveUDPAddr(networkUDP, addr)
	if err != nil {
		return nil, err
	}
	udpConn, err := net.ListenUDP(networkUDP, udpAddr)
	if err != nil {
		return nil, err
	}
	return udpConn, nil
}

func (j *JobHTTPServer) HTTP3Serve() error {

	if j.udpConn == nil {
		return errors.New("no udp connection")
	}
	defer j.udpConn.Close()

	quicConf := &quicgo.Config{}

	// setup debug trace
	if zerolog.GlobalLevel() <= zerolog.TraceLevel {
		quicConf.Tracer = utils.GenerateQLogTracer(j.Logger())
	}

	quicServer := &http3.Server{
		Addr:       j.Server.Addr,
		TLSConfig:  j.Server.TLSConfig,
		QUICConfig: quicConf,
		Handler:    j.Server.Handler,
	}

	h := j.Server.Handler
	j.Server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := quicServer.SetQUICHeaders(w.Header())
		j.Logger().Debug().Msgf("SetQuicHeaders called")
		if err != nil {
			j.Logger().Fatal().Err(err)
		}
		h.ServeHTTP(w, r)
	})

	// this blocks or fail
	j.Logger().Info().Msgf("nspeed serving https://%s (http3 on udp)", j.Server.Addr)

	j.Shutdown = func(shutdownTimeout time.Duration) {
		j.Logger().Debug().Msgf("")
		lctx, lcancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer lcancel()
		if err := quicServer.Shutdown(lctx); err != nil {
			j.Logger().Debug().Msgf("GO-QUIC server shutdown error: %s", err)
		}
	}
	return quicServer.Serve(j.udpConn)
}
