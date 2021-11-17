package nspeed

import (
	"math/rand"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

// mostly from go-httbin - todo: refactor / rewrite this

// metaResponseWriter implements http.ResponseWriter and http.Flusher in order
// to record a response's status code and body size for logging purposes.
type metaResponseWriter struct {
	w      http.ResponseWriter
	status int
	size   int64
}

func (mw *metaResponseWriter) Write(b []byte) (int, error) {
	size, err := mw.w.Write(b)
	mw.size += int64(size)
	return size, err
}

func (mw *metaResponseWriter) WriteHeader(s int) {
	mw.w.WriteHeader(s)
	mw.status = s
}

func (mw *metaResponseWriter) Flush() {
	f := mw.w.(http.Flusher)
	f.Flush()
}

func (mw *metaResponseWriter) Header() http.Header {
	return mw.w.Header()
}

func (mw *metaResponseWriter) Status() int {
	if mw.status == 0 {
		return http.StatusOK
	}
	return mw.status
}

func (mw *metaResponseWriter) Size() int64 {
	return mw.size
}

func logger(logger *zerolog.Logger, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mw := &metaResponseWriter{w: w}
		t := time.Now()
		h.ServeHTTP(mw, r)
		logger.Info().
			Int("status", mw.Status()).
			Str("method", r.Method).
			Stringer("URL", r.URL).
			Int64("size", mw.Size()).
			Dur("duration", time.Since(t))
	})
}

// limit inbound request size
func limitRequestSize(maxSize int64, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxSize)
		}
		h.ServeHTTP(w, r)
	})
}

func limitNumRequests(OnRequestStart func() bool, OnRequestEnd func(), h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if OnRequestStart() {
			h.ServeHTTP(w, r)
		} else {
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
		}
		OnRequestEnd()
	})
}

// headResponseWriter implements http.ResponseWriter in order to discard the
// body of the response
type headResponseWriter struct {
	*metaResponseWriter
}

func (hw *headResponseWriter) Write(b []byte) (int, error) {
	return 0, nil
}

// autohead automatically discards the body of responses to HEAD requests
func autohead(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w = &headResponseWriter{&metaResponseWriter{w: w}}
		}
		h.ServeHTTP(w, r)
	})
}

// todo: check if this is still needed,
// todo: add a CORS setting
func preflight(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		respHeader := w.Header()
		respHeader.Set("Access-Control-Allow-Origin", origin)
		respHeader.Set("Access-Control-Allow-Credentials", "true")
		respHeader.Set("Server:", ServerName+" "+ServerVersion)

		if r.Method == "OPTIONS" {
			w.Header().Set("Access-Control-Allow-Methods", "HEAD, GET, POST, OPTIONS") // , HEAD, PUT, DELETE, PATCH, ") // todo
			w.Header().Set("Access-Control-Max-Age", "3600")                           //todo
			if r.Header.Get("Access-Control-Request-Headers") != "" {
				w.Header().Set("Access-Control-Allow-Headers", r.Header.Get("Access-Control-Request-Headers"))
			}
			w.WriteHeader(200)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// BigBuffer is used with io.CopyBuffer because of io.Copy iternal buffer is too small.
// todo: we set a 4 MiB size. on small systems/embedded systems this could be an issue
// this buffer can be read/write by multiple goroutines at the same time.
// do not use the data in it.
var bigBuffer [4 * 1024 * 1024]byte
var BigBuffer = bigBuffer[:]

// BigChunk is a big array of random data
const MaxChunkSize = 1024 * 1024 // warning : 1 MiB //this will be allocated in memory
var BigChunk [MaxChunkSize]byte

// init a buffer of random values - callable only once
// thread safe
func InitBigChunk(seed int64) {
	rng := rand.New(rand.NewSource(seed))
	for i := int64(0); i < MaxChunkSize; i++ {
		BigChunk[i] = byte(rng.Intn(256))
	}
}

// called once
func init() {
	InitBigChunk(time.Now().Unix())
}
