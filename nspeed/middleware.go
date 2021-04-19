package nspeed

import (
	"errors"
	"fmt"
	"log"
	"net"
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

// ResolveInterfaceHostAddress returns the IP address(es) og 'host' by
// resolving 'host' as, in that order,: interface name, dns name
// if 'host' is a valid inteface name, all IPs of that interface are returned, IPv6 link-local(s) are last in the result
// if 'host' is a not valid interface, 'host' is treated as a dns name and resolved as such
func ResolveInterfaceHostAddress(host string) ([]net.IPAddr, error) {
	itf, err := net.InterfaceByName(host)
	if err != nil {
		ips, err := net.LookupIP(host)
		if err != nil {
			return nil, err
		}
		fmt.Printf("got dns for %s\n", host)
		var ipaddrs []net.IPAddr
		for _, ip := range ips {
			if ip == nil {
				continue
			}
			ipaddr := net.IPAddr{IP: ip}
			ipaddrs = append(ipaddrs, ipaddr)
		}
		return ipaddrs, nil
	}

	fmt.Printf("got interface for %s\n", host)
	// got an interface
	if itf.Flags&net.FlagUp == 0 {
		return nil, errors.New("interface is not up: " + itf.Name)
	}

	addrs, err := itf.Addrs()
	if err != nil {
		return nil, err
	}
	var ipaddrs []net.IPAddr
	for _, addr := range addrs {
		ipn, ok := addr.(*net.IPNet)
		if !ok {
			fmt.Printf("not IPAddr %s\n", addr.String())
			continue
		}
		ipa := net.IPAddr{IP: ipn.IP}
		if ipa.IP == nil {
			continue
		}
		// if linklocal & IPv6 -> move at the end
		if ipa.IP.IsLinkLocalUnicast() && ipa.IP.To4() == nil {
			ipa.Zone = itf.Name // todo: not sure this works on all platform, might use itf.Index
			ipaddrs = append(ipaddrs, ipa)
		} else {
			ipaddrs = append([]net.IPAddr{ipa}, ipaddrs...)
		}

	}
	return ipaddrs, nil
}
