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

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/http2"
)

// a HTTP command, get or post
// implement Job interface
type HTTPjob struct {
	Command          string
	Repeat           int
	IPversion        int
	Host             string
	Target           string
	Instance         int
	Timeout          time.Duration
	UserAgent        string
	ContentType      string
	ContentSize      int64
	SkipTLSCheck     bool
	UseTLSSelfConfig bool
	Delay            time.Duration
	Logger           *zerolog.Logger
	UseHTTP11        bool
	H2C              bool
}

func (j *HTTPjob) GoString() string {
	s := fmt.Sprintf("{Command: %s, URL: %q, IPversion: %d, Address: %s, Instance: %d, timeout: %s",
		j.Command,
		j.Target,
		j.IPversion,
		j.Host,
		j.Instance,
		j.Timeout.String())
	if j.Command == "POST" {
		s += fmt.Sprintf(",size: %d (%sB)", j.ContentSize, ByteCountDecimal(j.ContentSize))
	}
	s += "}"
	return s
}

func (j *HTTPjob) GetTarget() string {
	return j.Target
}

func (j *HTTPjob) PrintString() string {
	if j.Command == "GET" {
		return "get " + j.Target
	} else if j.Command == "POST" {
		return "put " + j.Target + " " + ByteCountDecimal(j.ContentSize) + "B"
	} else {
		return "unknown command"
	}
}

func (j *HTTPjob) Expand(jobs []Job) []Job {
	for i := 0; i < j.Repeat; i++ {
		clone := *j // use deep copy if struct change ?
		clone.Instance = i
		jobs = append(jobs, &clone)
	}
	return jobs
}

func (j *HTTPjob) IsInfinite() bool { return false }

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

	j.Logger = &wm.Logger

	var LocalAddr net.Addr
	if j.Host != "" {
		la, err := ResolveInterfaceHostAddress(j.Host)
		//la, err := net.ResolveIPAddr("ip", j.Host)
		if err != nil {
			return errors.New("bad local host address: " + j.Host)
		}

		fhostIPAddrs := FilterAddresses(la, j.IPversion)

		if len(fhostIPAddrs) == 0 {
			j.Logger.Error().Msgf("ResolveInterfaceHostAddress/FilterAddresses : %v/%v,%d", la, fhostIPAddrs, j.IPversion)
			return errors.New("no valid candidate address found")
		}

		LocalAddr = &net.TCPAddr{
			IP:   fhostIPAddrs[0].IP,
			Port: 0,
			Zone: fhostIPAddrs[0].Zone,
		}
	}

	var dialer = &net.Dialer{
		Timeout:       5 * time.Second, // fail quick
		FallbackDelay: -1,              // don't use Happy Eyeballs
		LocalAddr:     LocalAddr,       // "-a" for client
		// -4/-6 flags: there should a be higher level to enforce an IP version but for now we do this here
		Control: func(network string, address string, c syscall.RawConn) error {
			j.Logger.Debug().Msgf("Control %s %s %d ", network, address, j.IPversion)
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
		ForceAttemptHTTP2:   !j.UseHTTP11,
		TLSHandshakeTimeout: 5 * time.Second,
		DisableCompression:  true,
		// WriteBufferSize:     1024 * 1024,
		// ReadBufferSize:      1024 * 1024,
	}

	if j.UseTLSSelfConfig {
		netTransport.TLSClientConfig = TLSSelfConfig.Clone()
	}
	if j.SkipTLSCheck {
		netTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	if netTransport.TLSClientConfig != nil {
		log.Debug().Msgf("TLSConfig: defined")
	}

	var rtTransport http.RoundTripper = netTransport

	if j.H2C {
		rtTransport = &http2.Transport{
			AllowHTTP:          true,
			DisableCompression: netTransport.DisableCompression,
			DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
				return dialer.Dial(network, addr)
			},
		}
	}

	// create http client
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			loc, err := req.Response.Location()
			j.Logger.Info().Msgf("job %d got redirect to %s (%v)", wm.Index, loc.String(), err)

			wm.InitNew() //todo: evaluate
			return nil   // http.ErrUseLastResponse
		},
		Transport: rtTransport, // http.DefaultTransport,
		// Timeout:   10 * time.Second,
	}

	var body io.ReadCloser
	body = http.NoBody
	if j.Command == "POST" {
		wm.ContentWriteSize = j.ContentSize
		wm.ContentReadSize = 0
		wm.StepSize = 1024 * 1024 // todo
		wm.TotalWrite = 0
		wm.TotalWriteProcessed = 0
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

	// tracing & timing stuff
	refdate := time.Now()

	tr := &roundTripTrace{}

	// todo handle redirects
	trace := &httptrace.ClientTrace{
		DNSStart: func(dnsInfo httptrace.DNSStartInfo) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			tr.start = time.Now()
			wm.Logger.Debug().Msgf("DNSStart: %v", tr.start.Sub(refdate))
		},
		DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			wm.DNSInfoTime = time.Now()
			wm.DNSInfo = dnsInfo
			tr.dnsDone = wm.DNSInfoTime
			wm.Logger.Debug().Msgf("DNSDone: %v", wm.DNSInfoTime.Sub(refdate))
		},
		GotConn: func(connInfo httptrace.GotConnInfo) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			wm.ConnInfoTime = time.Now()
			wm.ConnInfo = connInfo
			tr.gotConn = wm.ConnInfoTime
			wm.Logger.Debug().Msgf("GotConn: %v", wm.ConnInfoTime.Sub(refdate))
		},
		GotFirstResponseByte: func() {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			wm.FirstResponseByte = time.Now()
			tr.responseStart = wm.FirstResponseByte
			wm.Logger.Debug().Msgf("GotFirstResponseByte: %v", wm.FirstResponseByte.Sub(refdate))
		},

		TLSHandshakeStart: func() {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			tr.tlsStart = time.Now()
			wm.Logger.Debug().Msgf("TLSHandshakeStart: %v", tr.tlsStart.Sub(refdate))
		},

		TLSHandshakeDone: func(t tls.ConnectionState, err error) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			if err == nil {
				tr.tls = true
				wm.TLSTime = time.Now()
				wm.TLSDone = true
				tr.tlsDone = wm.TLSTime
				wm.TLS = t
			} else {
				tr.tls = false
				fmt.Printf("tls: %+v \n", err)
			}
			wm.Logger.Debug().Msgf("TLSHandshakeDone: %v", wm.TLSTime.Sub(refdate))
		},
		ConnectStart: func(_, _ string) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			// No DNS resolution means we used an IP address directly.
			if tr.dnsDone.IsZero() {
				tr.start = time.Now()
				tr.dnsDone = tr.start
			}
			wm.Logger.Debug().Msgf("ConnectStart: %v", time.Since(refdate))
		},

		ConnectDone: func(net, addr string, err error) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			tr.connectDone = time.Now()
			wm.Logger.Debug().Msgf("ConnectDone: %v", tr.connectDone.Sub(refdate))
		},

		Got1xxResponse: func(code int, header textproto.MIMEHeader) error {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			j.Logger.Debug().Msgf("got 1xx: %d %+v", code, header)
			return nil
		},
	}

	// this is where we wait
	if j.Delay > 0 {
		j.Logger.Debug().Dur("delay", j.Delay).Msg("waiting")
		time.Sleep(j.Delay)
		j.Logger.Debug().Msg("waking up")
	}

	// add timeout
	if j.Timeout > 0 {
		thisctx, cancel := context.WithTimeout(ctx, j.Timeout)
		defer cancel()
		req = req.WithContext(thisctx)
	}

	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	wm.InitNew()

	// start the main timer
	refdate = time.Now()
	// resp, err := client.Transport.RoundTrip(req)
	resp, err := client.Do(req)
	tr.end = time.Now()

	if errors.Is(err, context.DeadlineExceeded) {
		wm.Expired = true
		j.Logger.Info().Msgf("timer expired client side")
	} else if errors.Is(err, syscall.ECONNRESET) {
		wm.Expired = true
		j.Logger.Info().Msgf("something expired server side")
	} else if err != nil {
		j.Logger.Fatal().Err(err).Send()
	}

	j.Logger.Info().
		Stringer("LocalAddr", wm.ConnInfo.Conn.LocalAddr()).
		Stringer("RemoteAddr", wm.ConnInfo.Conn.RemoteAddr()).
		Dur("DNSInfoTime", tr.dnsDone.Sub(refdate)). //wm.DNSInfoTime.Sub(refdate),
		Dur("ConnInfoTime", wm.ConnInfoTime.Sub(refdate)).
		Dur("FirstResponseByte", wm.FirstResponseByte.Sub(refdate)).
		Send()

	for _, a := range wm.DNSInfo.Addrs {
		j.Logger.Info().Stringer("dnsinfo", &a).Send()
	}

	if wm.ConnInfo.Reused {
		log.Info().Msg("REUSING CONNECTION")
	}

	if wm.TLSDone {
		j.Logger.Info().
			Str("crypto", TLSVersionName(wm.TLS.Version)).
			Str("cipher", tls.CipherSuiteName(wm.TLS.CipherSuite)).
			Dur("TLSDuration", wm.TLSTime.Sub(refdate)).Send()
	}

	// todo: time'd out 'POST' don't have a resp so we're missing informations like protocol ,tls, etc
	// todo: find a way to get the missing informations elsewhere
	// workaround: do a HEAD before the POST but that will count has 2 requests and could mess the server stats and Maxrequests option
	if resp != nil {
		protocol := resp.Proto
		j.Logger.Info().
			Str("protocol", protocol).
			Send()
		j.Logger.Debug().
			Int64("ContentLength", resp.ContentLength).
			Str("ContentLength-Decimal", ByteCountDecimal(resp.ContentLength)+"B").
			Str("ContentLength-Binary", ByteCountBinary(resp.ContentLength)+"B").
			Send()

		if resp.StatusCode != http.StatusOK {
			wm.Error = errors.New("http status code: " + resp.Status)
		} else {
			if j.Command == "GET" {
				defer func() {
					if resp.Body != nil {
						resp.Body.Close()
					}
				}()

				wm.ContentReadSize = resp.ContentLength

				// this is where the GET transfert happens.
				//   iteration 1: we use io.Copy to 'null' (Discard) and
				//   a TeeReader to do progression with the wm which has an io.Writer interface
				//   the resp (thru req) context  will handle timeout & parent cancellation

				wm.TotalReadProcessed, err = io.Copy(ioutil.Discard, io.TeeReader(resp.Body, wm))

				if err == context.DeadlineExceeded {
					wm.Expired = true
					j.Logger.Info().Str("cause", "timer expired client side")
				} else if err == io.ErrUnexpectedEOF {
					wm.Expired = true
					j.Logger.Info().Str("cause", "something expired server side")
				} else if err != nil {
					j.Logger.Error().Err(err)
					wm.Valid = false
					wm.Error = err
				} else {
					j.Logger.Info().Str("cause", "completed normally")
				}
			}
			//j.Logger.Info().Msgf("processed read:%d / write: %d bytes", wm.TotalReadProcessed, wm.TotalWriteProcessed)
			wm.Valid = true
		}
	}
	wm.LogFinal(j.Logger)
	wm.ResultChan <- wm
	return nil
}
