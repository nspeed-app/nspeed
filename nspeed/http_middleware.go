package nspeed

import (
	"math/rand"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

// mostly from go-httbin - todo: refactor / rewrite this

func logger(logger *zerolog.Logger, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//todo: this breaks websocket
		t := time.Now()
		h.ServeHTTP(w, r)
		logger.Info().
			Str("method", r.Method).
			Stringer("URL", r.URL).
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
		respHeader.Set("Server", ServerName+" "+ServerVersion) //todo:move this elsewhere ?

		if r.Method == "OPTIONS" {
			w.Header().Set("Access-Control-Allow-Methods", "HEAD, GET, POST, PUT, OPTIONS") // , HEAD, PUT, DELETE, PATCH, ") // todo
			w.Header().Set("Access-Control-Max-Age", "3600")                                //todo
			if r.Header.Get("Access-Control-Request-Headers") != "" {
				w.Header().Set("Access-Control-Allow-Headers", r.Header.Get("Access-Control-Request-Headers"))
			}
			w.WriteHeader(200)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// BigBuffer is used with io.CopyBuffer because of io.Copy internal buffer is too small.
// todo: we set a 4 MiB size. on small systems/embedded systems this could be an issue
// This buffer can be read/written by multiple goroutines at the same time so do not use the data in it.
// This will generate a data race
// io.CopyBuffer default size is 32 * 1024
var bigBuffer [1024 * 1024]byte
var BigBuffer = bigBuffer[:]

// BigChunk is a big array of random data
const MaxChunkSize = 1 * 1024 * 1024 // warning : 1 MiB //this will be allocated in memory
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
