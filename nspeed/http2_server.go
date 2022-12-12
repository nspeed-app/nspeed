package nspeed

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/net/http2"
)

const (
	runHooks  = true
	skipHooks = false
)

const debugServerConnections = false

type contextKey struct {
	name string
}

func (k *contextKey) String() string { return "http2 context value " + k.name }

var ServerContextKey = &contextKey{"http2-server"}
var LocalAddrContextKey = &contextKey{"local-addr"}

type atomicBool int32

func (b *atomicBool) isSet() bool { return atomic.LoadInt32((*int32)(b)) != 0 }

// func (b *atomicBool) setTrue()    { atomic.StoreInt32((*int32)(b), 1) }
// func (b *atomicBool) setFalse()   { atomic.StoreInt32((*int32)(b), 0) }

type HTTP2Server struct {
	http2.Server
	server        *http.Server
	inShutdown    atomicBool // true when server is in shutdown
	mu            sync.Mutex
	listeners     map[*net.Listener]struct{}
	doneChan      chan struct{}
	onShutdown    []func()
	activeConn    map[*conn]struct{}
	listenerGroup sync.WaitGroup
}

func (s *HTTP2Server) logf(format string, args ...any) {
	if s.server.ErrorLog != nil {
		s.server.ErrorLog.Printf(format, args...)
	} else {
		log.Printf(format, args...)
	}
}

func (s *HTTP2Server) shuttingDown() bool {
	return s.inShutdown.isSet()
}

func (s *HTTP2Server) getDoneChan() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getDoneChanLocked()
}

func (s *HTTP2Server) getDoneChanLocked() chan struct{} {
	if s.doneChan == nil {
		s.doneChan = make(chan struct{})
	}
	return s.doneChan
}

func (s *HTTP2Server) closeDoneChanLocked() {
	ch := s.getDoneChanLocked()
	select {
	case <-ch:
		// Already closed. Don't close again.
	default:
		// Safe to close here. We're the only closer, guarded
		// by s.mu.
		close(ch)
	}
}

func (s *HTTP2Server) trackListener(ln *net.Listener, add bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listeners == nil {
		s.listeners = make(map[*net.Listener]struct{})
	}
	if add {
		if s.shuttingDown() {
			return false
		}
		s.listeners[ln] = struct{}{}
		s.listenerGroup.Add(1)
	} else {
		delete(s.listeners, ln)
		s.listenerGroup.Done()
	}
	return true
}

// onceCloseListener wraps a net.Listener, protecting it from
// multiple Close calls.
type onceCloseListener struct {
	net.Listener
	once     sync.Once
	closeErr error
}

func (oc *onceCloseListener) Close() error {
	oc.once.Do(oc.close)
	return oc.closeErr
}

func (oc *onceCloseListener) close() { oc.closeErr = oc.Listener.Close() }
func (s *JobHTTPServer) HTTP2serve() error {
	// can't do unless we rewrite Serve because of we can't add
	// our own proto handler for http/1.1 (see )

	s2 := &HTTP2Server{server: s.Server}
	err := http2.ConfigureServer(s.Server, &s2.Server)

	if err != nil {
		fmt.Printf("error: %s\n", err)
		return err
	}

	idx := IndexOf(s.Server.TLSConfig.NextProtos, "http/1.1")
	if idx != -1 {
		log.Debug().Msgf("removing http/1.1 from server.TLSConfig")
		s.Server.TLSConfig.NextProtos = append(s.Server.TLSConfig.NextProtos[:idx], s.Server.TLSConfig.NextProtos[idx+1:]...)
	}

	config := s.Server.TLSConfig.Clone()

	s.Logger.Debug().Msgf("listening with %v\n", config.NextProtos)
	l := tls.NewListener(*s.listener, config)

	origListener := l
	l = &onceCloseListener{Listener: l}
	defer l.Close()

	if !s2.trackListener(&l, true) {
		return http.ErrServerClosed
	}
	defer s2.trackListener(&l, false)

	baseCtx := context.Background()
	if s.Server.BaseContext != nil {
		baseCtx = s.Server.BaseContext(origListener)
		if baseCtx == nil {
			panic("BaseContext returned a nil context")
		}
	}

	var tempDelay time.Duration // how long to sleep on accept failure

	ctx := context.WithValue(baseCtx, ServerContextKey, s2)
	for {
		rw, err := l.Accept()
		if err != nil {
			select {
			case <-s2.getDoneChan():
				return http.ErrServerClosed
			default:
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() { // was Temporary()
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				fmt.Printf("http: Accept error: %v; retrying in %v", err, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return err
		}
		connCtx := ctx
		if cc := s.Server.ConnContext; cc != nil {
			connCtx = cc(connCtx, rw)
			if connCtx == nil {
				panic("ConnContext returned nil")
			}
		}
		tempDelay = 0
		c := s2.newConn(rw)
		c.setState(c.rwc, http.StateNew, runHooks) // before Serve can return
		go c.serve(connCtx)
	}

	//return s.Server.Serve(tlsListener)
}

// Create new connection from rwc.
func (srv *HTTP2Server) newConn(rwc net.Conn) *conn {
	c := &conn{
		server: srv,
		rwc:    rwc,
	}
	if debugServerConnections {
		c.rwc = newLoggingConn("server", c.rwc)
	}
	return c
}

// ErrAbortHandler is a sentinel panic value to abort a handler.
// While any panic from ServeHTTP aborts the response to the client,
// panicking with ErrAbortHandler also suppresses logging of a stack
// trace to the server's error log.
var ErrAbortHandler = errors.New("net/http: abort Handler")

func (c *conn) serve(ctx context.Context) {
	c.remoteAddr = c.rwc.RemoteAddr().String()
	ctx = context.WithValue(ctx, LocalAddrContextKey, c.rwc.LocalAddr())
	var inFlightResponse *response
	defer func() {
		if err := recover(); err != nil && err != ErrAbortHandler {
			const size = 64 << 10
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)]
			c.server.logf("http: panic serving %v: %v\n%s", c.remoteAddr, err, buf)
		}
		if inFlightResponse != nil {
			inFlightResponse.cancelCtx()
		}
		if !c.hijacked() {
			if inFlightResponse != nil {
				inFlightResponse.conn.r.abortPendingRead()
				inFlightResponse.reqBody.Close()
			}
			c.close()
			c.setState(c.rwc, http.StateClosed, runHooks)
		}
	}()

	if tlsConn, ok := c.rwc.(*tls.Conn); ok {
		tlsTO := c.server.tlsHandshakeTimeout()
		if tlsTO > 0 {
			dl := time.Now().Add(tlsTO)
			c.rwc.SetReadDeadline(dl)
			c.rwc.SetWriteDeadline(dl)
		}
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			// If the handshake failed due to the client not speaking
			// TLS, assume they're speaking plaintext HTTP and write a
			// 400 response on the TLS conn's underlying net.Conn.
			if re, ok := err.(tls.RecordHeaderError); ok && re.Conn != nil && tlsRecordHeaderLooksLikeHTTP(re.RecordHeader) {
				io.WriteString(re.Conn, "HTTP/1.0 400 Bad Request\r\n\r\nClient sent an HTTP request to an HTTPS server.\n")
				re.Conn.Close()
				return
			}
			c.server.logf("http: TLS handshake error from %s: %v", c.rwc.RemoteAddr(), err)
			return
		}
		// Restore Conn-level deadlines.
		if tlsTO > 0 {
			c.rwc.SetReadDeadline(time.Time{})
			c.rwc.SetWriteDeadline(time.Time{})
		}
		c.tlsState = new(tls.ConnectionState)
		*c.tlsState = tlsConn.ConnectionState()
		proto := c.tlsState.NegotiatedProtocol
		//fmt.Println("version", TLSVersionFriendlyName(c.tlsState.Version))
		c.server.logf("http/2: ALPN from %s: %s", c.rwc.RemoteAddr(), proto)
		if fn := c.server.server.TLSNextProto[proto]; fn != nil {
			h := initALPNRequest{ctx, tlsConn, serverHandler{c.server}}
			// Mark freshly created HTTP/2 as active and prevent any server state hooks
			// from being run on these connections. This prevents closeIdleConns from
			// closing such connections. See issue https://golang.org/issue/39776.
			c.setState(c.rwc, http.StateActive, skipHooks)
			fmt.Println("calling ALPN fn for", proto)
			fn(c.server.server, tlsConn, h)
			return
		}
		io.WriteString(c.rwc, "HTTP/1.1 505 HTTP Version Not Supported\r\n\r\nunsupported protocol\n")
	}
	return
}

// A conn represents the server side of an HTTP connection.
type conn struct {
	// server is the server on which the connection arrived.
	// Immutable; never nil.
	server *HTTP2Server

	// cancelCtx cancels the connection-level context.
	cancelCtx context.CancelFunc

	// rwc is the underlying network connection.
	// This is never wrapped by other types and is the value given out
	// to CloseNotifier callers. It is usually of type *net.TCPConn or
	// *tls.Conn.
	rwc net.Conn

	// remoteAddr is rwc.RemoteAddr().String(). It is not populated synchronously
	// inside the Listener's Accept goroutine, as some implementations block.
	// It is populated immediately inside the (*conn).serve goroutine.
	// This is the value of a Handler's (*Request).RemoteAddr.
	remoteAddr string

	// tlsState is the TLS connection state when using TLS.
	// nil means not TLS.
	tlsState *tls.ConnectionState

	// werr is set to the first write error to rwc.
	// It is set via checkConnErrorWriter{w}, where bufw writes.
	werr error

	// r is bufr's read source. It's a wrapper around rwc that provides
	// io.LimitedReader-style limiting (while reading request headers)
	// and functionality to support CloseNotifier. See *connReader docs.
	r *connReader

	// bufr reads from r.
	bufr *bufio.Reader

	// bufw writes to checkConnErrorWriter{c}, which populates werr on error.
	bufw *bufio.Writer

	// lastMethod is the method of the most recent request
	// on this connection, if any.
	lastMethod string

	curReq atomic.Value // of *response (which has a Request in it)

	curState struct{ atomic uint64 } // packed (unixtime<<8|uint8(ConnState))

	// mu guards hijackedv
	mu sync.Mutex

	// hijackedv is whether this connection has been hijacked
	// by a Handler with the Hijacker interface.
	// It is guarded by mu.
	hijackedv bool
}

func (c *conn) setState(nc net.Conn, state http.ConnState, runHook bool) {
	srv := c.server
	switch state {
	case http.StateNew:
		srv.trackConn(c, true)
	case http.StateHijacked, http.StateClosed:
		srv.trackConn(c, false)
	}
	if state > 0xff || state < 0 {
		panic("internal error")
	}
	packedState := uint64(time.Now().Unix()<<8) | uint64(state)
	atomic.StoreUint64(&c.curState.atomic, packedState)
	if !runHook {
		return
	}
	if hook := srv.server.ConnState; hook != nil {
		hook(nc, state)
	}
}

func (s *HTTP2Server) trackConn(c *conn, add bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeConn == nil {
		s.activeConn = make(map[*conn]struct{})
	}
	if add {
		s.activeConn[c] = struct{}{}
	} else {
		delete(s.activeConn, c)
	}
}

// tlsHandshakeTimeout returns the time limit permitted for the TLS
// handshake, or zero for unlimited.
//
// It returns the minimum of any positive ReadHeaderTimeout,
// ReadTimeout, or WriteTimeout.
func (srv *HTTP2Server) tlsHandshakeTimeout() time.Duration {
	var ret time.Duration
	for _, v := range [...]time.Duration{
		srv.server.ReadHeaderTimeout,
		srv.server.ReadTimeout,
		srv.server.WriteTimeout,
	} {
		if v <= 0 {
			continue
		}
		if ret == 0 || v < ret {
			ret = v
		}
	}
	return ret
}

// connReader is the io.Reader wrapper used by *conn. It combines a
// selectively-activated io.LimitedReader (to bound request header
// read sizes) with support for selectively keeping an io.Reader.Read
// call blocked in a background goroutine to wait for activity and
// trigger a CloseNotifier channel.
type connReader struct {
	conn *conn

	mu      sync.Mutex // guards following
	hasByte bool
	byteBuf [1]byte
	cond    *sync.Cond
	inRead  bool
	aborted bool  // set true before conn.rwc deadline is set to past
	remain  int64 // bytes remaining
}

func (cr *connReader) lock() {
	cr.mu.Lock()
	if cr.cond == nil {
		cr.cond = sync.NewCond(&cr.mu)
	}
}

func (cr *connReader) unlock() { cr.mu.Unlock() }

// aLongTimeAgo is a non-zero time, far in the past, used for
// immediate cancellation of network operations.
var aLongTimeAgo = time.Unix(1, 0)

func (cr *connReader) abortPendingRead() {
	cr.lock()
	defer cr.unlock()
	if !cr.inRead {
		return
	}
	cr.aborted = true
	cr.conn.rwc.SetReadDeadline(aLongTimeAgo)
	for cr.inRead {
		cr.cond.Wait()
	}
	cr.conn.rwc.SetReadDeadline(time.Time{})
}

// maxInt64 is the effective "infinite" value for the Server and
// Transport's byte-limiting readers.
const maxInt64 = 1<<63 - 1

func (cr *connReader) setReadLimit(remain int64) { cr.remain = remain }
func (cr *connReader) setInfiniteReadLimit()     { cr.remain = maxInt64 }
func (cr *connReader) hitReadLimit() bool        { return cr.remain <= 0 }

func (c *conn) hijacked() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hijackedv
}

var (
	bufioReaderPool   sync.Pool
	bufioWriter2kPool sync.Pool
	bufioWriter4kPool sync.Pool
)

func putBufioReader(br *bufio.Reader) {
	br.Reset(nil)
	bufioReaderPool.Put(br)
}

func (c *conn) finalFlush() {
	if c.bufr != nil {
		// Steal the bufio.Reader (~4KB worth of memory) and its associated
		// reader for a future connection.
		putBufioReader(c.bufr)
		c.bufr = nil
	}

	if c.bufw != nil {
		c.bufw.Flush()
		// Steal the bufio.Writer (~4KB worth of memory) and its associated
		// writer for a future connection.
		putBufioWriter(c.bufw)
		c.bufw = nil
	}
}

// Close the connection.
func (c *conn) close() {
	c.finalFlush()
	c.rwc.Close()
}

// loggingConn is used for debugging.
type loggingConn struct {
	name string
	net.Conn
}

var (
	uniqNameMu   sync.Mutex
	uniqNameNext = make(map[string]int)
)

func newLoggingConn(baseName string, c net.Conn) net.Conn {
	uniqNameMu.Lock()
	defer uniqNameMu.Unlock()
	uniqNameNext[baseName]++
	return &loggingConn{
		name: fmt.Sprintf("%s-%d", baseName, uniqNameNext[baseName]),
		Conn: c,
	}
}

// A response represents the server side of an HTTP response.
type response struct {
	conn             *conn
	req              *http.Request // request for this response
	reqBody          io.ReadCloser
	cancelCtx        context.CancelFunc // when ServeHTTP exits
	wroteHeader      bool               // a non-1xx header has been (logically) written
	wroteContinue    bool               // 100 Continue response was written
	wants10KeepAlive bool               // HTTP/1.0 w/ Connection "keep-alive"
	wantsClose       bool               // HTTP request has Connection "close"

	// canWriteContinue is a boolean value accessed as an atomic int32
	// that says whether or not a 100 Continue header can be written
	// to the connection.
	// writeContinueMu must be held while writing the header.
	// These two fields together synchronize the body reader
	// (the expectContinueReader, which wants to write 100 Continue)
	// against the main writer.
	canWriteContinue atomicBool
	writeContinueMu  sync.Mutex

	w  *bufio.Writer // buffers output in chunks to chunkWriter
	cw chunkWriter

	// handlerHeader is the Header that Handlers get access to,
	// which may be retained and mutated even after WriteHeader.
	// handlerHeader is copied into cw.header at WriteHeader
	// time, and privately mutated thereafter.
	handlerHeader http.Header
	calledHeader  bool // handler accessed handlerHeader via Header

	written       int64 // number of bytes written in body
	contentLength int64 // explicitly-declared Content-Length; or -1
	status        int   // status code passed to WriteHeader

	// close connection after this reply.  set on request and
	// updated after response from handler if there's a
	// "Connection: keep-alive" response header and a
	// Content-Length.
	closeAfterReply bool

	// requestBodyLimitHit is set by requestTooLarge when
	// maxBytesReader hits its max size. It is checked in
	// WriteHeader, to make sure we don't consume the
	// remaining request body to try to advance to the next HTTP
	// request. Instead, when this is set, we stop reading
	// subsequent requests on this connection and stop reading
	// input from it.
	requestBodyLimitHit bool

	// trailers are the headers to be sent after the handler
	// finishes writing the body. This field is initialized from
	// the Trailer response header when the response header is
	// written.
	trailers []string

	handlerDone atomicBool // set true when the handler exits

	// Buffers for Date, Content-Length, and status code
	dateBuf   [len(http.TimeFormat)]byte
	clenBuf   [10]byte
	statusBuf [3]byte

	// closeNotifyCh is the channel returned by CloseNotify.
	// TODO(bradfitz): this is currently (for Go 1.8) always
	// non-nil. Make this lazily-created again as it used to be?
	closeNotifyCh  chan bool
	didCloseNotify int32 // atomic (only 0->1 winner should send)
}

// chunkWriter writes to a response's conn buffer, and is the writer
// wrapped by the response.w buffered writer.
//
// chunkWriter also is responsible for finalizing the Header, including
// conditionally setting the Content-Type and setting a Content-Length
// in cases where the handler's final output is smaller than the buffer
// size. It also conditionally adds chunk headers, when in chunking mode.
//
// See the comment above (*response).Write for the entire write flow.
type chunkWriter struct {
	res *response

	// header is either nil or a deep clone of res.handlerHeader
	// at the time of res.writeHeader, if res.writeHeader is
	// called and extra buffering is being done to calculate
	// Content-Type and/or Content-Length.
	header http.Header

	// wroteHeader tells whether the header's been written to "the
	// wire" (or rather: w.conn.buf). this is unlike
	// (*response).wroteHeader, which tells only whether it was
	// logically written.
	wroteHeader bool

	// set by the writeHeader method:
	chunking bool // using chunked transfer encoding for reply body
}

// serverHandler delegates to either the server's Handler or
// DefaultServeMux and also handles "OPTIONS *" requests.
type serverHandler struct {
	srv *HTTP2Server
}

func (sh serverHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	handler := sh.srv.server.Handler
	if handler == nil {
		handler = http.DefaultServeMux
	}
	if req.RequestURI == "*" && req.Method == "OPTIONS" {
		handler = globalOptionsHandler{}
	}

	if req.URL != nil && strings.Contains(req.URL.RawQuery, ";") {
		var allowQuerySemicolonsInUse int32
		req = req.WithContext(context.WithValue(req.Context(), silenceSemWarnContextKey, func() {
			atomic.StoreInt32(&allowQuerySemicolonsInUse, 1)
		}))
		defer func() {
			if atomic.LoadInt32(&allowQuerySemicolonsInUse) == 0 {
				sh.srv.logf("http: URL query contains semicolon, which is no longer a supported separator; parts of the query may be stripped when parsed; see golang.org/issue/25192")
			}
		}()
	}

	handler.ServeHTTP(rw, req)
}

var silenceSemWarnContextKey = &contextKey{"silence-semicolons"}

// initALPNRequest is an HTTP handler that initializes certain
// uninitialized fields in its *Request. Such partially-initialized
// Requests come from ALPN protocol handlers.
type initALPNRequest struct {
	ctx context.Context
	c   *tls.Conn
	h   serverHandler
}

func (h initALPNRequest) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if req.TLS == nil {
		req.TLS = &tls.ConnectionState{}
		*req.TLS = h.c.ConnectionState()
	}
	if req.Body == nil {
		req.Body = http.NoBody
	}
	if req.RemoteAddr == "" {
		req.RemoteAddr = h.c.RemoteAddr().String()
	}
	h.h.ServeHTTP(rw, req)
}

// tlsRecordHeaderLooksLikeHTTP reports whether a TLS record header
// looks like it might've been a misdirected plaintext HTTP request.
func tlsRecordHeaderLooksLikeHTTP(hdr [5]byte) bool {
	switch string(hdr[:]) {
	case "GET /", "HEAD ", "POST ", "PUT /", "OPTIO":
		return true
	}
	return false
}

func bufioWriterPool(size int) *sync.Pool {
	switch size {
	case 2 << 10:
		return &bufioWriter2kPool
	case 4 << 10:
		return &bufioWriter4kPool
	}
	return nil
}

func putBufioWriter(bw *bufio.Writer) {
	bw.Reset(nil)
	if pool := bufioWriterPool(bw.Available()); pool != nil {
		pool.Put(bw)
	}
}

// globalOptionsHandler responds to "OPTIONS *" requests.
type globalOptionsHandler struct{}

func (globalOptionsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Length", "0")
	if r.ContentLength != 0 {
		// Read up to 4KB of OPTIONS body (as mentioned in the
		// spec as being reserved for future use), but anything
		// over that is considered a waste of server resources
		// (or an attack) and we abort and close the connection,
		// courtesy of MaxBytesReader's EOF behavior.
		mb := http.MaxBytesReader(w, r.Body, 4<<10)
		io.Copy(io.Discard, mb)
	}
}
