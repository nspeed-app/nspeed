package nspeed

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

// a HTTP command, get or post
// implement Job interface
type HTTPjob struct {
	Command      string
	Repeat       int
	IPversion    int
	Host         string
	Target       string
	Instance     int
	Timeout      time.Duration
	UserAgent    string
	ContentType  string
	ContentSize  int64
	SkipTLSCheck bool
}

func (j *HTTPjob) GoString() string {
	return fmt.Sprintf("%s {URL: %q, IPversion: %d, Host: %s, Instance: %d, timeout: %s}",
		j.Command,
		j.Target,
		j.IPversion,
		j.Host,
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

	var LocalAddr net.Addr
	if j.Host != "" {
		la, err := ResolveInterfaceHostAddress(j.Host)
		//la, err := net.ResolveIPAddr("ip", j.Host)
		if err != nil {
			return errors.New("bad local host address: " + j.Host)
		}

		fhostIPAddrs := FilterAddresses(la, j.IPversion)

		if len(fhostIPAddrs) == 0 {
			log.Errorf("ResolveInterfaceHostAddress/FilterAddresses : %v/%v,%d", la, fhostIPAddrs, j.IPversion)
			return errors.New("no valid candidate address found")
		}

		LocalAddr = &net.TCPAddr{
			IP:   fhostIPAddrs[0].IP,
			Port: 0,
			Zone: fhostIPAddrs[0].Zone,
		}
	}

	var dialer = &net.Dialer{
		Timeout:       5 * time.Second,
		FallbackDelay: -1,
		LocalAddr:     LocalAddr, // "-a" for client
		//there should a be higher level to enforce an IP version but for now we do this here
		Control: func(network string, address string, c syscall.RawConn) error {
			log.Debugf("Control %s %s %d ", network, address, j.IPversion)
			if j.IPversion == 4 && network[len(network)-1] == '6' {
				return errors.New("bad IP version: want IPv4 got " + network)
			}
			if j.IPversion == 6 && network[len(network)-1] == '4' {
				return errors.New("bad IP version: want IPv6 got " + network)
			}
			return nil
		},
	}

	var netTransport = &http.Transport{
		DialContext:         dialer.DialContext,
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: 5 * time.Second,
		// WriteBufferSize:     1024 * 1024,
		// ReadBufferSize:      1024 * 1024,
	}

	if j.SkipTLSCheck {
		netTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	// create http client
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			loc, err := req.Response.Location()
			log.Infof("job %d got redirect to %s (%v)", wm.Index, loc.String(), err)

			wm.InitNew() //todo: evaluate
			return nil   // http.ErrUseLastResponse
		},
		Transport: netTransport, // http.DefaultTransport,
		// Timeout:   10 * time.Second,
	}

	var body io.ReadCloser
	body = http.NoBody
	if j.Command == "POST" {
		wm.ContentSize = j.ContentSize
		wm.StepSize = 1024 * 1024 // todo
		wm.TotalRead = 0
		wm.TotalProcessed = 0
		body = wm
	}
	req, err := http.NewRequestWithContext(ctx, j.Command, wm.Target, body)
	if err != nil {
		return err
	}

	// add more headers here
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", j.UserAgent)
	req.Header.Set("Content-Type", j.ContentType)

	tr := &roundTripTrace{}

	// todo handle redirects
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
			// No DNS resolution means we used an IP address directly.
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

		Got1xxResponse: func(code int, header textproto.MIMEHeader) error {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			log.Infof("got 1xx: %d %+v", code, header)
			return nil
		},
	}

	refdate := time.Now()

	// add timeout
	if j.Timeout > 0 {
		thisctx, cancel := context.WithTimeout(ctx, j.Timeout)
		defer cancel()
		req = req.WithContext(thisctx)
	}

	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	wm.InitNew()

	// resp, err := client.Transport.RoundTrip(req)
	resp, err := client.Do(req)
	tr.end = time.Now()

	if errors.Is(err, context.DeadlineExceeded) {
		wm.Expired = true
		log.Info("#", wm.Index, " timer expired client side")
	} else if errors.Is(err, syscall.ECONNRESET) {
		wm.Expired = true
		log.Info("#", wm.Index, " something expired server side")
	} else if err != nil {
		log.Fatal(err)
	}

	if j.Command == "GET" && resp != nil {
		defer func() {
			if resp.Body != nil {
				resp.Body.Close()
			}
		}()

		protocol := resp.Proto
		log.Info("#", wm.Index,
			" size = ", ByteCountDecimal(resp.ContentLength)+"B", " (", ByteCountBinary(resp.ContentLength)+"B", ")",
			" procotol = ", protocol,
			" local IP:port = ", wm.ConnInfo.Conn.LocalAddr(),
			" remote IP:port = ", wm.ConnInfo.Conn.RemoteAddr(),
			" DNSInfoTime = ", tr.dnsDone.Sub(refdate), //wm.DNSInfoTime.Sub(refdate),
			" ConnInfoTime = ", wm.ConnInfoTime.Sub(refdate),
			" FirstResponseByte = ", wm.FirstResponseByte.Sub(refdate),
		)
		for _, a := range wm.DNSInfo.Addrs {
			log.Info("#", wm.Index, "dns: ", a)
		}
		if resp.TLS != nil {
			log.Info("#", wm.Index,
				" crypto = ", TLSVersionName(resp.TLS.Version),
				" cipher = ", tls.CipherSuiteName(resp.TLS.CipherSuite),
				" TLSDone in = ", wm.TLSDone.Sub(refdate),
			)
		}

		wm.ContentSize = resp.ContentLength

		// this is where the GET transfert happens.
		//   iteration 1: we use io.Copy to 'null' (Discard) and
		//   a TeeReader to do progression with the wm which has an io.Writer interface
		//   the resp (thru req) context  will handle timeout & parent cancellation

		if resp.StatusCode == http.StatusOK {
			if wm.TotalProcessed, err = io.Copy(ioutil.Discard, io.TeeReader(resp.Body, wm)); err != nil {
				if err == context.DeadlineExceeded {
					wm.Expired = true
					log.Info("#", wm.Index, " timer expired client side")
				} else if err == io.ErrUnexpectedEOF {
					wm.Expired = true
					log.Info("#", wm.Index, " something expired server side")
				} else {
					log.Error("#", wm.Index, " error: ", err)
					wm.Valid = false
					wm.Error = err
				}
			}
			log.Info("#", wm.Index, " transfered ", wm.TotalProcessed, " bytes")
			wm.Valid = true
		} else {
			wm.Error = errors.New("http status code: " + resp.Status)
		}
	}
	wm.ResultChan <- wm
	return nil
}
