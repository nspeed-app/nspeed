package nspeed

import (
	"crypto/tls"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"time"

	log "github.com/sirupsen/logrus"
)

// HTTPDownload implements HTTP GET command
func HTTPDownload(wm *WorkerMetrics) error {

	// create http client
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: http.DefaultTransport,
	}

	// A - temporay dns debug
	if wm.Verbose {
		u, err := url.Parse(wm.URL)
		if err != nil {
			panic(err)
		}

		host, _, err := net.SplitHostPort(u.Host)
		if err != nil {
			host = u.Host
		}

		ips, err := net.LookupIP(host)

		log.Debug("LookupIp for ", host)
		for _, ip := range ips {
			log.Debug("  ", ip.String())
		}
		// end of A
	}
	// Get the data
	req, err := http.NewRequest("GET", wm.URL, nil)

	// add headers
	//req.Header.Add("x-nspeed-app-version", "1.0")
	//"accept: application/octet-stream"

	// todo: CheckRedirect

	if err != nil {
		return err
	}

	//

	trace := &httptrace.ClientTrace{
		DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
			wm.DNSInfoTime = time.Now()
			wm.DNSInfo = dnsInfo
		},
		GotConn: func(connInfo httptrace.GotConnInfo) {
			wm.ConnInfoTime = time.Now()
			wm.ConnInfo = connInfo
		},
		GotFirstResponseByte: func() {
			wm.FirstResponseByte = time.Now()
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	resp, err := client.Transport.RoundTrip(req)
	if err != nil {
		log.Fatal(err)
	}

	protocol := resp.Proto
	crypto := "n/a"
	cipher := "n/a"
	if resp.TLS != nil {
		crypto = TLSVersionName(resp.TLS.Version)
		cipher = tls.CipherSuiteName(resp.TLS.CipherSuite)
	}

	log.Info("#", wm.Index+1,
		" size = ", ByteCountDecimal(resp.ContentLength, "B"), " (", ByteCountBinary(resp.ContentLength, "iB"), ")",
		" procotol = ", protocol,
		" crypto = ", crypto,
		" cipher = ", cipher,
		" local IP:port", wm.ConnInfo.Conn.LocalAddr(),
		" remote IP:port", wm.ConnInfo.Conn.RemoteAddr())

	wm.StartTime = time.Now()
	wm.ContentSize = resp.ContentLength

	// this is where the main transfert happens.
	//   iteration 1: we use io.Copy to 'null' (Discard) and a TeeReader to do progression
	if _, err = io.Copy(ioutil.Discard, io.TeeReader(resp.Body, wm)); err != nil {
		return err
	}
	wm.ElapsedTime = time.Since(wm.StartTime)
	wm.Done = true
	wm.ResultChan <- wm

	return nil
}
