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
	"net/http/httputil"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/quic-go/qlog"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/http2"
	"nspeed.app/utils"
)

// this will be used to set a big value for MaxReadFrameSize (HTTP/2)
// see https://github.com/golang/go/issues/54850
const h2MaxFrameSize uint32 = 1 << 20

// roundTripTrace holds timings for a single HTTP roundtrip.
// greatly inspired by Prometheus blackbox_exporter
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
	DialTimeout      time.Duration
	UserAgent        string
	ContentType      string
	ContentSize      int64
	SkipTLSCheck     bool
	UseTLSSelfConfig bool
	Delay            time.Duration
	Logger           *zerolog.Logger
	HTTPVersion      int
	H2C              bool
	// private
	client      *http.Client
	req         *http.Request
	tr          roundTripTrace
	numRedirect int
	finalTarget string
	wm          *Metrics
	JobList     *JobList
}

func (j *JobHTTPClient) GoString() string {
	if zerolog.GlobalLevel() > zerolog.InfoLevel {
		return fmt.Sprintf("HTTP %s %s", j.Command, j.Target)
	}
	s := fmt.Sprintf("HTTP client {Method: %s, URL: %q, IPversion: %d, Address: %s, Group: %d, timeout: %s, delay: %s, h2c: %t, http:%s}",
		j.Command,
		j.Target,
		j.IPversion,
		j.Host,
		j.Group,
		j.Timeout.String(),
		j.Delay.String(),
		j.H2C,
		utils.HTTPVersionStringer(j.HTTPVersion),
	)
	if j.Command == "POST" || j.Command == "PUT" {
		s += fmt.Sprintf(",size: %d (%sB)", j.ContentSize, utils.ByteCountDecimal(j.ContentSize))
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
		details := fmt.Sprintf("%s (%s - %d.%d ms - %s)", r, j.GetDestination(), latency, latencyMicroRem, j.wm.Protocol)
		if j.numRedirect > 0 && verbose {
			return fmt.Sprintf("%s from %s", details, j.Target)
		}
		return details
	}

	if j.Command == "GET" {
		t := "get %s" + j.finalTarget
		r := fmt.Sprintf(t, utils.HTTPVersionFlag(j.HTTPVersion, j.H2C))
		return formatOut(r)

	} else if j.Command == "POST" || j.Command == "PUT" {
		t := strings.ToLower(j.Command) + " %s" + j.finalTarget + " " + utils.ByteCountDecimal(j.ContentSize) + "B" //todo
		r := fmt.Sprintf(t, utils.HTTPVersionFlag(j.HTTPVersion, j.H2C))
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

func (j *JobHTTPClient) ID() string { return "" }

func (j *JobHTTPClient) GetMetric() (NetMetric, error) {
	return NetMetric{}, fmt.Errorf("not yet implemented")
}

// init a HTTP Job
func (j *JobHTTPClient) Init(wm *Metrics) error {

	j.Logger = &wm.Logger
	j.wm = wm

	// prepend https scheme if no scheme
	if !strings.Contains(j.Target, "://") {
		j.Target = "https://" + j.Target
	}
	var err error
	j.finalTarget, err = ResolveSchemes(j.JobList.IDMap, j.Target)
	if err != nil {
		return err
	}
	j.Logger.Debug().Msgf("target is: %s", j.finalTarget)

	// source address
	var LocalAddr net.Addr
	if j.Host != "" {
		la, err := utils.ResolveInterfaceHostAddress(j.Host)
		//la, err := net.ResolveIPAddr("ip", j.Host)
		if err != nil {
			return errors.New("bad local host address: " + j.Host)
		}

		fhostIPAddrs := utils.FilterAddresses(la, j.IPversion)

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

	// dialer
	var dialer = &net.Dialer{
		Timeout:       j.DialTimeout, //
		FallbackDelay: -1,            // don't use Happy Eyeballs
		LocalAddr:     LocalAddr,     // "-a" for client
		// TODO: remove later
		Control: func(network string, address string, c syscall.RawConn) error {
			j.Logger.Debug().Msgf("Control %s %s %d ", network, address, j.IPversion)
			// old code, moved to transport.DialContext
			// if len(network) == 0 {
			// 	return nil
			// }
			// v := network[len(network)-1]
			// if j.IPversion == 4 && v == '6' {
			// 	return errors.New("bad IP version: want IPv4 got " + network)
			// }
			// if j.IPversion == 6 && v == '4' {
			// 	return errors.New("bad IP version: want IPv6 got " + network)
			// }
			return nil
		},
	}

	// we clone to avoid ConfigureTransports to crash
	//var netTransport = http.DefaultTransport.(*http.Transport).Clone()
	var netTransport = &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     j.HTTPVersion != 1, // what about -http3?
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    true,
		DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
			j.Logger.Info().Msgf("DialContext %s %s %d ", network, address, j.IPversion)
			if network == "udp" || network == "tcp" {
				// dont assume j.IPversion  is sanitized
				if j.IPversion == 6 || j.IPversion == 4 {
					network = network + strconv.Itoa(j.IPversion)
				}
				// else should fatal ?
			}
			return dialer.DialContext(ctx, network, address)
		},
	}

	if j.UseTLSSelfConfig {
		netTransport.TLSClientConfig = TLSSelfConfig.Clone()
		// netTransport.TLSClientConfig.CipherSuites = []uint16{tls.TLS_CHACHA20_POLY1305_SHA256}
		// netTransport.TLSClientConfig.MinVersion = tls.VersionTLS13
	}
	if j.SkipTLSCheck {
		netTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	if netTransport.TLSClientConfig != nil {
		j.Logger.Debug().Msgf("TLSConfig: defined")
	}

	// interface to the transport so we can switch if needed
	var rtTransport http.RoundTripper = netTransport

	if j.HTTPVersion != 1 {
		// get a http2 transport
		t2, err := http2.ConfigureTransports(netTransport)
		if err != nil {
			j.Logger.Warn().Msgf("something bad happened that shouldn't : %s", err)
			return err
		}
		// default settings of frame size impacts performance, so we bump it
		t2.MaxReadFrameSize = h2MaxFrameSize

		// force http/2 : this is hacky: remove "http/1.1" from NextProtos.
		// Forcing a http2.Transport fails unless it's h2c mode.
		if j.HTTPVersion == 2 {
			idx := utils.IndexOf(netTransport.TLSClientConfig.NextProtos, "http/1.1")
			if idx != -1 {
				log.Debug().Msgf("removing http/1.1 from netTransport.TLSClientConfig")
				netTransport.TLSClientConfig.NextProtos = append(netTransport.TLSClientConfig.NextProtos[:idx], netTransport.TLSClientConfig.NextProtos[idx+1:]...)
			}
		}
		if j.H2C {
			rtTransport = &http2.Transport{
				DialTLSContext: func(ctx context.Context, network string, addr string, cfg *tls.Config) (net.Conn, error) {
					return dialer.DialContext(ctx, network, addr)
				},
				TLSClientConfig:    netTransport.TLSClientConfig.Clone(),
				DisableCompression: true,
				AllowHTTP:          j.H2C,
				MaxReadFrameSize:   h2MaxFrameSize,
			}
		}
		if j.HTTPVersion == 3 {
			quicConf := &quic.Config{}
			if zerolog.GlobalLevel() <= zerolog.TraceLevel {
				quicConf.Tracer = qlog.NewTracer(utils.QLogTracer(j.Logger))
			}
			rtTransport = &http3.RoundTripper{
				DisableCompression: true,
				TLSClientConfig:    netTransport.TLSClientConfig,
				QuicConfig:         quicConf,
				Dial: func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (quic.EarlyConnection, error) {
					j.Logger.Debug().Msgf("HTTP3: %s,%v", addr, cfg)
					return quic.DialAddrEarlyContext(ctx, addr, tlsCfg, cfg)
				},
			}
		}
	}

	if j.HTTPVersion == 1 {
		// force HTTP1.1 as per https://pkg.go.dev/net/http#pkg-overview
		netTransport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
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

			wm.InitNew() //todo: evaluate - Body rewind may interactions (redirection happends afte consuming the body?)?
			j.numRedirect++
			j.finalTarget = loc.String()
			return nil // http.ErrUseLastResponse
		},
		Transport: rtTransport,
	}

	var body io.ReadCloser
	body = http.NoBody
	if j.Command == "POST" || j.Command == "PUT" {
		wm.ContentWriteSize = j.ContentSize
		wm.ContentReadSize = 0
		wm.StepSize = 256 * 1024 // todo:
		wm.TotalWrite = 0
		body = wm
	}

	req, err := http.NewRequest(j.Command, j.finalTarget, body)
	if err != nil {
		return err
	}

	// add more headers here
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", j.UserAgent)
	req.Header.Set("Content-Type", j.ContentType)

	// post,put: avoid chunked transfert which is default net/http client behavior.
	if j.Command == "POST" || j.Command == "PUT" {
		req.Header.Set("Expect", "100-continue")
		req.TransferEncoding = nil
		req.ContentLength = j.ContentSize
	}

	j.req = req
	return nil
}

func (j *JobHTTPClient) Run(ctx context.Context, wg *sync.WaitGroup) error {

	defer wg.Done()
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
			wm.Logger.Debug().Msgf("ClientTrace-DNSStart: %v", j.tr.start.Sub(refdate))
		},
		DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			j.tr.dnsDone = time.Now()
			j.tr.DNSInfo = dnsInfo
			wm.Logger.Debug().Msgf("ClientTrace-DNSDone: %v", j.tr.dnsDone.Sub(refdate))
		},
		GotConn: func(connInfo httptrace.GotConnInfo) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			j.tr.gotConn = time.Now()
			j.tr.ConnInfo = connInfo
			wm.Logger.Debug().Msgf("ClientTrace-GotConn: %v", j.tr.gotConn.Sub(refdate))
		},
		GotFirstResponseByte: func() {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			j.tr.responseStart = time.Now()
			wm.FirstResponseByte = j.tr.responseStart // todo: does this belong here ?
			wm.Logger.Debug().Msgf("ClientTrace-GotFirstResponseByte: %v", j.tr.responseStart.Sub(refdate))
		},

		TLSHandshakeStart: func() {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			j.tr.tlsStart = time.Now()
			wm.Logger.Debug().Msgf("ClientTrace-TLSHandshakeStart: %v", j.tr.tlsStart.Sub(refdate))
		},

		TLSHandshakeDone: func(t tls.ConnectionState, err error) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			if err == nil {
				j.tr.tls = true
				j.tr.tlsDone = time.Now()
				j.tr.TLSDone = true
				j.tr.TLS = t
				wm.Logger.Debug().Msgf("ClientTrace-TLSHandshakeDone: %v (protocol = %s)", j.tr.tlsDone.Sub(refdate), t.NegotiatedProtocol)
			} else {
				j.tr.tls = false
				//fmt.Printf("tls: %+v \n", err)
			}
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
			wm.Logger.Debug().Msgf("ClientTrace-onnectStart: %v", time.Since(refdate))
		},

		ConnectDone: func(net, addr string, err error) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			j.tr.connectDone = time.Now()
			wm.Logger.Debug().Msgf("ClientTrace-ConnectDone: %v", j.tr.connectDone.Sub(refdate))
		},

		Got1xxResponse: func(code int, header textproto.MIMEHeader) error {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			j.Logger.Debug().Msgf("ClientTrace-got 1xx: %d %+v", code, header)
			return nil
		},

		WroteHeaderField: func(key string, value []string) {
			j.Logger.Debug().Msgf("ClientTrace-WroteHeaderField %s = %v", key, value)
		},
		WroteHeaders: func() {
			j.Logger.Debug().Msgf("ClientTrace-WroteHeaders")
		},
		Wait100Continue: func() {
			j.Logger.Debug().Msgf("ClientTrace-Wait100Continue")
		},
		WroteRequest: func(wri httptrace.WroteRequestInfo) {
			j.wm.Logger.Debug().Err(wri.Err).Msgf("ClientTrace-WroteRequest")
			// todo: can we catch stuff here ?!
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
	wm.InitNew()

	utils.PrintMemUsage(j.Logger, "before Do")
	resp, err := j.client.Do(j.req)
	utils.PrintMemUsage(j.Logger, "after Do")

	j.Logger.Debug().Msgf("client.Do: %v - %v", err, wm.Error)
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
		//fmt.Printf("TR: j.tr %+v\n", j.tr)
		if j.tr.ConnInfo.Conn != nil {
			j.Logger.Info().
				Stringer("LocalAddr", j.tr.ConnInfo.Conn.LocalAddr()).
				Stringer("RemoteAddr", j.tr.ConnInfo.Conn.RemoteAddr()).Send()
		}
		j.Logger.Info().
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
				Str("crypto", utils.TLSVersionName(j.tr.TLS.Version)).
				Str("cipher", tls.CipherSuiteName(j.tr.TLS.CipherSuite)).
				Dur("TLSDuration", j.tr.tlsDone.Sub(refdate)).Send()
		}
	}
	// todo: time'd out 'POST/PUT' don't have a resp so we're missing informations like protocol ,tls, etc
	// todo: find a way to get the missing informations elsewhere
	// possible workaround: do a HEAD before the POST/PUT but that will count has 2 requests and could mess the server stats and Maxrequests option
	// or HEAD shouldn't count as a request server side ?
	if wm.Error == nil && resp != nil {
		wm.Protocol = resp.Proto
		j.Logger.Info().
			Str("protocol", wm.Protocol).
			Send()
		j.Logger.Debug().
			Int64("ContentLength", resp.ContentLength).
			Str("ContentLength-Decimal", utils.ByteCountDecimal(resp.ContentLength)+"B").
			Str("ContentLength-Binary", utils.ByteCountBinary(resp.ContentLength)+"B").
			Send()

		// dump resp if trace on
		if ShouldLog(j.Logger, zerolog.TraceLevel) {
			dump, err := httputil.DumpResponse(resp, false)
			if err != nil {
				j.Logger.Fatal().Err(err).Send()
			}
			j.Logger.Debug().Msgf("Response:\n%s", dump)
		}

		if resp.StatusCode != http.StatusOK {
			wm.Error = errors.New("http status code: " + resp.Status)
			j.Logger.Debug().Msg(resp.Status)
			wm.Valid = false
		} else {
			if j.Command == "GET" {
				defer func() {
					if resp.Body != nil {
						resp.Body.Close()
					}
				}()

				wm.ContentReadSize = resp.ContentLength

				// this is where the GET transfert happens.
				//   iteration 3: we use io.CopyBuffer the body to 'wm' (which is a Discard) and
				//   the resp (thru req) context will handle timeout & parent cancellation

				//wm.TotalReadProcessed, err = io.Copy(ioutil.Discard, io.TeeReader(resp.Body, wm))
				//todo: invesstigate why the following gives results too high
				totalread, err := io.CopyBuffer(wm, resp.Body, BigBuffer)

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
			// if (j.Command == "PUT" || j.Command == "POST") && j.Logger.GetLevel() <= zerolog.TraceLevel {

			// 	b, err := io.ReadAll(resp.Body)
			// 	if err != nil {
			// 		log.Error().Err(err).Msg("read response.Body")
			// 	}
			// 	n := len(b)
			// 	if n == 0 {
			// 		log.Error().Msg("empty body")
			// 	} else {
			// 		fmt.Println("HEAD OF BOBY----------------------------------------------------------------")
			// 		fmt.Println(string(b[:Min(1000, len(b)-1)]))
			// 		fmt.Println("----------------------------------------------------------------------------")
			// 	}
			// }

			wm.Valid = true
		}
	}
	utils.PrintMemUsage(j.Logger, "client ended")
	wm.LogFinal(j.Logger)
	wm.ResultChan <- wm
	return nil
}

// handle special schemes
func ResolveSchemes(idmap map[string]Job, target string) (string, error) {
	url, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	if url.Scheme == "nspeed" {
		t, ok := idmap[url.Host]
		if !ok {
			return "", fmt.Errorf("named job '%s' not found", url.Host)
		}
		s, sok := t.(*JobHTTPServer)
		if !sok {
			return "", fmt.Errorf("named job '%s' is not a server", url.Host)
		}
		if s.URL == nil {
			return "", fmt.Errorf("named job '%s' not running", url.Host)
		}
		url.Scheme = s.URL.Scheme
		url.Host = s.URL.Host
		return url.String(), nil
	}
	return target, nil
}
