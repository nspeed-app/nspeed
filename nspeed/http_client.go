package nspeed

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/http2"
)

// roundTripTrace holds timings for a single HTTP roundtrip.
// mostly from prometheus blackbox_exporter
type roundTripTrace struct {
	tls           bool
	start         time.Time
	dnsDone       time.Time
	tlsStart      time.Time
	tlsDone       time.Time
	connectStart  time.Time
	connectDone   time.Time
	gotConn       time.Time
	responseStart time.Time
	end           time.Time
	ConnInfo      httptrace.GotConnInfo
	DNSInfo       httptrace.DNSDoneInfo
	TLSDone       bool
	TLS           tls.ConnectionState
}

// a HTTP command, get or post
// implements Job interface
type JobHTTPClient struct {
	Command          string
	Repeat           int
	IPversion        int
	Host             string
	Target           string
	Group            int
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
	// private
	client      *http.Client
	req         *http.Request
	tr          roundTripTrace
	numRedirect int
	finalTarget string
	wm          *Metrics
}

func (j *JobHTTPClient) GoString() string {
	s := fmt.Sprintf("HTTP client {Method: %s, URL: %q, IPversion: %d, Address: %s, Group: %d, timeout: %s, delay: %s, h2c: %t, http1.1:%t}",
		j.Command,
		j.Target,
		j.IPversion,
		j.Host,
		j.Group,
		j.Timeout.String(),
		j.Delay.String(),
		j.H2C,
		j.UseHTTP11,
	)
	if j.Command == "POST" {
		s += fmt.Sprintf(",size: %d (%sB)", j.ContentSize, ByteCountDecimal(j.ContentSize))
	}
	s += "}"
	return s
}

func (j *JobHTTPClient) GetTarget() string {
	return j.Target
}

func (j *JobHTTPClient) GetConn() *httptrace.GotConnInfo {
	if j.wm != nil {
		return &j.tr.ConnInfo
	}
	return nil
}

func (j *JobHTTPClient) GetDestination() string {
	ci := j.GetConn()
	if ci == nil {
		return ""
	}
	if ci.Conn == nil {
		return ""
	}
	return ci.Conn.RemoteAddr().String()
}

// Pretty print command and some infos
func (j *JobHTTPClient) PrintString(verbose bool) string {

	formatOut := func(r string) string {
		latency := j.tr.connectDone.Sub(j.tr.connectStart).Milliseconds()
		latencyMicroRem := j.tr.connectDone.Sub(j.tr.connectStart).Microseconds() % 1000
		details := fmt.Sprintf("%s (%s - %d.%d ms)", r, j.GetDestination(), latency, latencyMicroRem)
		if j.numRedirect > 0 && verbose {
			return fmt.Sprintf("%s from %s", details, j.Target)
		}
		return details
	}

	if j.Command == "GET" {
		t := "get %s" + j.finalTarget
		r := fmt.Sprintf(t, "")
		if j.H2C {
			r = fmt.Sprintf(t, "-h2c ")
		}
		if j.UseHTTP11 {
			r = fmt.Sprintf(t, "-http11 ")
		}
		return formatOut(r)

	} else if j.Command == "POST" {
		t := "put %s" + j.finalTarget + " " + ByteCountDecimal(j.ContentSize) + "B"
		r := fmt.Sprintf(t, "")
		if j.H2C {
			r = fmt.Sprintf(t, "-h2c ")
		}
		if j.UseHTTP11 {
			r = fmt.Sprintf(t, "-http11 ")
		}
		return formatOut(r)
	} else {
		return "unknown command"
	}
}

func (j *JobHTTPClient) Expand(jobs []Job, group int) []Job {
	for i := 0; i < j.Repeat; i++ {
		clone := *j // use deep copy if struct change ?
		clone.Group = group
		jobs = append(jobs, &clone)
	}
	return jobs
}

func (j *JobHTTPClient) IsInfinite() bool { return false }

// init a HTTP Job
func (j *JobHTTPClient) Init(wm *Metrics) error {

	j.Logger = &wm.Logger
	j.wm = wm

	// prepend http scheme if no scheme
	if !strings.Contains(j.Target, "://") {
		j.Target = "http://" + j.Target
	}
	j.finalTarget = j.Target
	j.Logger.Debug().Msgf("target parsed to: %s", j.Target)

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
		// WriteBufferSize:     10 * 1024 * 1024,
		// ReadBufferSize:      10 * 1024 * 1024,
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

	// interface to the transport so we can switch if needed
	var rtTransport http.RoundTripper = netTransport

	// H2C support, beware: we're not using netTransport anymore
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
	j.client = &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			loc, err := req.Response.Location()
			if err != nil {
				j.Logger.Info().Msgf("redirected with error %v", err)
			} else {
				j.Logger.Info().Msgf("redirected to %s", loc.String())
			}

			wm.InitNew() //todo: evaluate
			j.numRedirect++
			j.finalTarget = loc.String()
			return nil // http.ErrUseLastResponse
		},
		Transport: rtTransport, // http.DefaultTransport,
		// Timeout:   10 * time.Second,
	}

	var body io.ReadCloser
	body = http.NoBody
	if j.Command == "POST" {
		wm.ContentWriteSize = j.ContentSize
		wm.ContentReadSize = 0
		wm.StepSize = 1024 * 1024 // todo:
		wm.TotalWrite = 0
		body = wm
	}
	req, err := http.NewRequest(j.Command, j.Target, body)
	if err != nil {
		return err
	}

	j.req = req
	// add more headers here
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", j.UserAgent)
	req.Header.Set("Content-Type", j.ContentType)

	wm.InitNew()
	return nil
}

func (j *JobHTTPClient) Run(ctx context.Context) error {

	if j.req == nil {
		return errors.New("no request")
	}

	if ctx != nil {
		j.req = j.req.WithContext(ctx)
	}

	wm := j.wm
	// tracing & timing stuff
	refdate := time.Now()

	// todo handle redirects more nicely (array ?)
	trace := &httptrace.ClientTrace{
		DNSStart: func(dnsInfo httptrace.DNSStartInfo) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			j.tr.start = time.Now()
			wm.Logger.Debug().Msgf("DNSStart: %v", j.tr.start.Sub(refdate))
		},
		DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			j.tr.dnsDone = time.Now()
			j.tr.DNSInfo = dnsInfo
			wm.Logger.Debug().Msgf("DNSDone: %v", j.tr.dnsDone.Sub(refdate))
		},
		GotConn: func(connInfo httptrace.GotConnInfo) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			j.tr.gotConn = time.Now()
			j.tr.ConnInfo = connInfo
			wm.Logger.Debug().Msgf("GotConn: %v", j.tr.gotConn.Sub(refdate))
		},
		GotFirstResponseByte: func() {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			j.tr.responseStart = time.Now()
			wm.FirstResponseByte = j.tr.responseStart // todo: does this belong here ?
			wm.Logger.Debug().Msgf("GotFirstResponseByte: %v", j.tr.responseStart.Sub(refdate))
		},

		TLSHandshakeStart: func() {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			j.tr.tlsStart = time.Now()
			wm.Logger.Debug().Msgf("TLSHandshakeStart: %v", j.tr.tlsStart.Sub(refdate))
		},

		TLSHandshakeDone: func(t tls.ConnectionState, err error) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			if err == nil {
				j.tr.tls = true
				j.tr.tlsDone = time.Now()
				j.tr.TLSDone = true
				j.tr.TLS = t
			} else {
				j.tr.tls = false
				//fmt.Printf("tls: %+v \n", err)
			}
			wm.Logger.Debug().Msgf("TLSHandshakeDone: %v", j.tr.tlsDone.Sub(refdate))
		},
		ConnectStart: func(_, _ string) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			// No DNS resolution means we used an IP address directly.
			j.tr.connectStart = time.Now()
			if j.tr.dnsDone.IsZero() {
				j.tr.start = j.tr.connectStart
				j.tr.dnsDone = j.tr.start
			}
			wm.Logger.Debug().Msgf("ConnectStart: %v", time.Since(refdate))
		},

		ConnectDone: func(net, addr string, err error) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			j.tr.connectDone = time.Now()
			wm.Logger.Debug().Msgf("ConnectDone: %v", j.tr.connectDone.Sub(refdate))
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
		j.req = j.req.WithContext(thisctx)
	}

	j.req = j.req.WithContext(httptrace.WithClientTrace(j.req.Context(), trace))

	// start the main timer
	refdate = time.Now()
	// resp, err := client.Transport.RoundTrip(req)
	resp, err := j.client.Do(j.req)
	j.tr.end = time.Now()

	if errors.Is(err, context.DeadlineExceeded) {
		wm.Expired = true
		j.Logger.Info().Msgf("timer expired client side")
	} else if errors.Is(err, syscall.ECONNRESET) {
		wm.Expired = true
		j.Logger.Info().Msgf("something expired server side")
	} else if errors.Is(err, context.Canceled) {
		wm.Expired = true
		wm.Cancelled = true
		j.Logger.Info().Msgf("parent got cancelled")
	} else if err != nil {
		wm.Error = err
		j.Logger.Info().Err(err).Send()
		// todo: get body for more info ()
	}

	// completed or expired (timeout)
	if wm.Error == nil && !wm.Cancelled {
		j.Logger.Info().
			Stringer("LocalAddr", j.tr.ConnInfo.Conn.LocalAddr()).
			Stringer("RemoteAddr", j.tr.ConnInfo.Conn.RemoteAddr()).
			Dur("DNSInfoTime", j.tr.dnsDone.Sub(refdate)). //wm.DNSInfoTime.Sub(refdate),
			Dur("ConnInfoTime", j.tr.gotConn.Sub(refdate)).
			Dur("FirstResponseByte", wm.FirstResponseByte.Sub(refdate)).
			Send()

		for _, a := range j.tr.DNSInfo.Addrs {
			j.Logger.Info().Stringer("dnsinfo", &a).Send()
		}

		if j.tr.ConnInfo.Reused {
			log.Info().Msg("REUSING CONNECTION")
		}

		if j.tr.TLSDone {
			j.Logger.Info().
				Str("crypto", TLSVersionName(j.tr.TLS.Version)).
				Str("cipher", tls.CipherSuiteName(j.tr.TLS.CipherSuite)).
				Dur("TLSDuration", j.tr.tlsDone.Sub(refdate)).Send()
		}
	}
	// todo: time'd out 'POST' don't have a resp so we're missing informations like protocol ,tls, etc
	// todo: find a way to get the missing informations elsewhere
	// possible workaround: do a HEAD before the POST but that will count has 2 requests and could mess the server stats and Maxrequests option
	if wm.Error == nil && resp != nil {
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
				//   iteration 2: we use io.Copy the body to 'wm' (which is a Discard) and
				//   the resp (thru req) context  will handle timeout & parent cancellation

				//wm.TotalReadProcessed, err = io.Copy(ioutil.Discard, io.TeeReader(resp.Body, wm))
				//todo: invesstigate why the following gives results too high
				totalread, err := io.Copy(wm, resp.Body)

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
					if totalread != wm.TotalRead {
						j.Logger.Warn().Msgf("iocopy result differs from interface: %d - %d", totalread, wm.TotalRead)
					}
				}
			}
			wm.Valid = true
		}
	}
	wm.LogFinal(j.Logger)
	wm.ResultChan <- wm
	return nil
}
