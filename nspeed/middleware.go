package nspeed

import (
	"fmt"
	"log"
	"net/http"
	"time"
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

func logger(o func(Result), h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mw := &metaResponseWriter{w: w}
		t := time.Now()
		h.ServeHTTP(mw, r)
		o(Result{
			Status:   mw.Status(),
			Method:   r.Method,
			URI:      r.URL.RequestURI(),
			Size:     mw.Size(),
			Duration: time.Since(t),
		})
	})
}

func limitRequestSize(maxSize int64, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxSize)
		}
		h.ServeHTTP(w, r)
	})
}

func limitNumRequests(OnRequestEnded func(), h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
		OnRequestEnded()
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
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS") // , HEAD, PUT, DELETE, PATCH, ") // todo
			w.Header().Set("Access-Control-Max-Age", "3600")                     //todo
			if r.Header.Get("Access-Control-Request-Headers") != "" {
				w.Header().Set("Access-Control-Allow-Headers", r.Header.Get("Access-Control-Request-Headers"))
			}
			w.WriteHeader(200)
			return
		}

		// debug - dump headers
		for k, v := range r.Header {
			fmt.Printf("%v: %v\n", k, v)
		}

		h.ServeHTTP(w, r)
	})
}

// StdLogFunc creates a func that will log each request in structured
// format using the given stdlib logger
func StdLogFunc(l *log.Logger) func(Result) {
	const (
		logFmt  = "time=%q status=%d method=%q uri=%q size_bytes=%d duration_ms=%0.02f"
		dateFmt = "2006-01-02T15:04:05.9999"
	)
	return func(result Result) {
		l.Printf(
			logFmt,
			time.Now().Format(dateFmt),
			result.Status,
			result.Method,
			result.URI,
			result.Size,
			result.Duration.Seconds()*1e3, // https://github.com/golang/go/issues/5491#issuecomment-66079585
		)
	}
}
