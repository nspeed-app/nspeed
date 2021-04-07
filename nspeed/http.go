package nspeed

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"time"

	log "github.com/sirupsen/logrus"
)

// a HTTP command, get or post
// implement Job interface
type HTTPjob struct {
	Command   string
	Repeat    int
	IPversion int
	Target    string
	Instance  int
	Timeout   time.Duration
}

func (j *HTTPjob) GoString() string {
	return fmt.Sprintf("%s {URL: %q, IPversion: %d, Instance: %d, timeout: %s}",
		j.Command,
		j.Target,
		j.IPversion,
		j.Instance,
		j.Timeout.String(),
	)
}

func (j *HTTPjob) GetTarget() string {
	return j.Target
}

func (j *HTTPjob) Expand(jobs []Job) []Job {
	for i := 0; i < j.Repeat; i++ {
		clone := *j // use deep copy if struct change ?
		clone.Instance = i
		jobs = append(jobs, &clone)
	}
	return jobs
}

// roundTripTrace holds timings for a single HTTP roundtrip.
// mostly from prometheus blackbox_exporter
type roundTripTrace struct {
	tls           bool
	start         time.Time
	dnsDone       time.Time
	tlsStart      time.Time
	tlsDone       time.Time
	connectDone   time.Time
	gotConn       time.Time
	responseStart time.Time
	end           time.Time
}

// HTTPDownload implements HTTP GET command
func (j *HTTPjob) Start(ctx context.Context, wm *WorkerMetrics) error {

	// create http client
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: http.DefaultTransport,
	}

	// A - temporay dns debug
	if wm.Verbose {
		u, err := url.Parse(wm.Target)
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
	req, err := http.NewRequestWithContext(ctx, "GET", wm.Target, nil)

	// add headers
	//req.Header.Add("x-nspeed-app-version", "1.0")
	//"accept: application/octet-stream"

	// todo: CheckRedirect

	if err != nil {
		return err
	}

	tr := &roundTripTrace{}

	trace := &httptrace.ClientTrace{
		DNSStart: func(dnsInfo httptrace.DNSStartInfo) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			tr.start = time.Now()
		},
		DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			wm.DNSInfoTime = time.Now()
			wm.DNSInfo = dnsInfo
			tr.dnsDone = wm.DNSInfoTime
		},
		GotConn: func(connInfo httptrace.GotConnInfo) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			wm.ConnInfoTime = time.Now()
			wm.ConnInfo = connInfo
			tr.gotConn = wm.ConnInfoTime
		},
		GotFirstResponseByte: func() {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			wm.FirstResponseByte = time.Now()
			tr.responseStart = wm.FirstResponseByte
		},

		TLSHandshakeStart: func() {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			tr.tlsStart = time.Now()
		},

		TLSHandshakeDone: func(t tls.ConnectionState, err error) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			if err == nil {
				tr.tls = true
				wm.TLSDone = time.Now()
				tr.tlsDone = wm.TLSDone
			} else {
				tr.tls = false
				fmt.Printf("tls: %+v \n", err)
			}
		},
		ConnectStart: func(_, _ string) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			// No DNS resolution because we connected to IP directly.
			if tr.dnsDone.IsZero() {
				tr.start = time.Now()
				tr.dnsDone = tr.start
			}
		},

		ConnectDone: func(net, addr string, err error) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			tr.connectDone = time.Now()
		},
	}

	refdate := time.Now()

	// add timeout (we do this just before the body consumption)
	if j.Timeout > 0 {
		thisctx, cancel := context.WithTimeout(ctx, j.Timeout)
		defer cancel()
		req = req.WithContext(thisctx)
	}

	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	resp, err := client.Transport.RoundTrip(req)
	tr.end = time.Now()
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

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
		" remote IP:port", wm.ConnInfo.Conn.RemoteAddr(),
		" DNSInfoTime = ", wm.DNSInfoTime.Sub(refdate),
		" ConnInfoTime = ", wm.ConnInfoTime.Sub(refdate),
		" FirstResponseByte = ", wm.FirstResponseByte.Sub(refdate),
		" TLSDone = ", wm.TLSDone.Sub(refdate),
	)

	// fmt.Println("start", tr.start.Sub(refdate))
	// fmt.Println("dnsDone", tr.dnsDone.Sub(refdate))
	// fmt.Println("tlsStart", tr.tlsStart.Sub(refdate))
	// fmt.Println("tlsDone", tr.tlsDone.Sub(refdate))
	// fmt.Println("connectDone", tr.connectDone.Sub(refdate))
	// fmt.Println("gotConn", tr.gotConn.Sub(refdate))
	// fmt.Println("responseStart", tr.responseStart.Sub(refdate))
	// fmt.Println("end", tr.end.Sub(refdate))

	wm.TestStartTime = time.Now()
	wm.ContentSize = resp.ContentLength

	// this is where the main transfert happens.
	//   iteration 1: we use io.Copy to 'null' (Discard) and
	//   a TeeReader to do progression with the wm which has an io.Writer interface
	//   the resp (thru req) context  will handle timeout & parent cancellation

	if wm.TotalProcessed, err = io.Copy(ioutil.Discard, io.TeeReader(resp.Body, wm)); err != nil {
		if err == context.DeadlineExceeded {
			wm.Expired = true
			log.Info("#", wm.Index+1, " expired client side")
		} else if err == io.ErrUnexpectedEOF {
			wm.Expired = true
			log.Info("#", wm.Index+1, " expired server side")
		} else {
			log.Error("#", wm.Index+1, " error: ", err)
			return err
		}
	}
	log.Info("#", wm.Index+1, " copied ", wm.TotalProcessed)
	wm.ElapsedTime = time.Since(wm.TestStartTime)
	wm.Done = true
	wm.ResultChan <- wm

	return nil
}
