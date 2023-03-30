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
	"nspeed.app/webview"
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
	JobData
	Command          string
	Repeat           int
	IPversion        int
	Host             string
	Target           string
	Timeout          time.Duration
	DialTimeout      time.Duration
	UserAgent        string
	ContentType      string
	ContentSize      int64
	SkipTLSCheck     bool
	UseTLSSelfConfig bool
	Delay            time.Duration
	HTTPVersion      int
	H2C              bool
	jobList          *JobList
	// private
	client      *http.Client
	req         *http.Request
	tr          roundTripTrace
	numRedirect int
	finalTarget string
	wm          *Metric
}

func (j *JobHTTPClient) GoString() string {
	if zerolog.GlobalLevel() > zerolog.InfoLevel {
		return fmt.Sprintf("HTTP %s %s", j.Command, j.Target)
	}
	return j.CommandString()
}
func (j *JobHTTPClient) CommandString() string {
	s := fmt.Sprintf("HTTP client {Method: %s, URL: %s, IPversion: %d, Address: %s, timeout: %s, delay: %s, h2c: %t, http:%s}",
		j.Command,
		j.Target,
		j.IPversion,
		j.Host,
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
// todo: fix me
func (j *JobHTTPClient) PrintString(verbose bool, group bool) string {

	formatOut := func(r string) string {
		latency := j.tr.connectDone.Sub(j.tr.connectStart).Milliseconds()
		latencyMicroRem := j.tr.connectDone.Sub(j.tr.connectStart).Microseconds() % 1000
		ipv := utils.IPVersion(j.GetDestination())
		details := fmt.Sprintf("%s (%s - %d.%d ms - %s - %s)", r, ipv, latency, latencyMicroRem, j.wm.Protocol, j.wm.TLSVersion)
		if j.numRedirect > 0 && verbose {
			return fmt.Sprintf("%s from %s", details, j.Target)
		}
		return details
	}

	groupFormat := ""
	if group {
		groupFormat = "-n " + strconv.Itoa(j.Repeat) + " "
	}
	if j.Command == "GET" {
		t := "get " + groupFormat + "%s" + j.finalTarget
		r := fmt.Sprintf(t, utils.HTTPVersionFlag(j.HTTPVersion, j.H2C))
		return formatOut(r)

	} else if j.Command == "POST" || j.Command == "PUT" {
		t := strings.ToLower(j.Command) + " " + groupFormat + "%s" + j.finalTarget + " " + utils.ByteCountDecimal(j.ContentSize) + "B" //todo
		r := fmt.Sprintf(t, utils.HTTPVersionFlag(j.HTTPVersion, j.H2C))
		return formatOut(r)
	} else {
		return "unknown command"
	}
}

func (j *JobHTTPClient) IsInfinite() bool { return false }

// init a HTTP Job
func (j *JobHTTPClient) Init() error {

	// todo move this to run ? and get ride of .wm?
	j.wm = &Metric{
		MetricsData: MetricsData{
			Done:    false,
			Expired: false,
			Target:  j.GetTarget(),
			ID:      j.ID(),
			GroupID: j.GroupID(),
			Command: j.CommandString(),
		},
		Logger: *j.Logger(),
		//Tick:       opt.Tick, fixme
	}

	// prepend https scheme if no scheme
	if !strings.Contains(j.Target, "://") {
		j.Target = "https://" + j.Target
	}
	var err error
	j.finalTarget, err = ResolveSchemes(j.jobList.Jobs, j.Target)
	if err != nil {
		return err
	}
	j.Logger().Info().Msgf("real target is: %s", j.finalTarget)

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
			j.Logger().Error().Msgf("ResolveInterfaceHostAddress/FilterAddresses : %v/%v,%d", la, fhostIPAddrs, j.IPversion)
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
		Control: func(network string, address string, c syscall.RawConn) error {
			j.Logger().Debug().Msgf("Control %s %s %d ", network, address, j.IPversion)
			var operr error
			if err := c.Control(func(fd uintptr) {
				buf, err := webview.GetSockOpt(fd, syscall.SO_RCVBUF)
				if err != nil {
					j.Logger().Info().Err(err).Msgf("getsockopt rcvbuf")
					return
				}
				j.Logger().Info().Msgf("getsockopt rcvbuf = %d", buf)
				buf, err = webview.GetSockOpt(fd, syscall.SO_SNDBUF)
				if err != nil {
					j.Logger().Info().Err(err).Msgf("getsockopt sndbuf")
					return
				}
				j.Logger().Info().Msgf("getsockopt sndbuf = %d", buf)
			}); err != nil {
				return err
			}
			return operr
		},
	}

	// we clone to avoid ConfigureTransports to crash
	// var netTransport = http.DefaultTransport.(*http.Transport).Clone()
	var netTransport = &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     j.HTTPVersion != 1, // what about -http3?
		MaxIdleConns:          100,
		IdleConnTimeout:       10 * time.Second, // add parameter for this ?
		TLSHandshakeTimeout:   5 * time.Second,  // this seems long ?
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    true,
		//  transforms network "tcp" / "udp" to "tcp4"/"udp4" or "tcp6"/"udp6" if IPVerson is set.
		DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
			j.Logger().Info().Msgf("DialContext %s %s %d ", network, address, j.IPversion)
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
		j.Logger().Info().Msgf("using TLSSelfConfig")
		// for tls 1.3 we can't change cipher suites with current Go implementation: https://go.dev/blog/tls-cipher-suites
		// so something like this won't work:
		// netTransport.TLSClientConfig.CipherSuites = []uint16{tls.TLS_CHACHA20_POLY1305_SHA256}
		// netTransport.TLSClientConfig.MinVersion = tls.VersionTLS13
	}
	if j.SkipTLSCheck {
		netTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		j.Logger().Info().Msgf("skipping TLS Check")
	}

	if netTransport.TLSClientConfig != nil {
		j.Logger().Debug().Msgf("TLSConfig: defined")
	}

	// interface to the transport so we can switch if needed
	var rtTransport http.RoundTripper = netTransport

	if j.HTTPVersion != 1 { // 1 means 1.11 too
		// get a http2 transport
		t2, err := http2.ConfigureTransports(netTransport)
		if err != nil {
			j.Logger().Warn().Msgf("something bad happened that shouldn't : %s", err)
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
				quicConf.Tracer = qlog.NewTracer(utils.QLogTracer(j.Logger()))
			}
			rtTransport = &http3.RoundTripper{
				DisableCompression: true,
				TLSClientConfig:    netTransport.TLSClientConfig,
				QuicConfig:         quicConf,
				Dial: func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (quic.EarlyConnection, error) {
					j.Logger().Debug().Msgf("HTTP3: %s,%v", addr, cfg)
					return quic.DialAddrEarlyContext(ctx, addr, tlsCfg, cfg)
				},
			}
		}
	}

	if j.HTTPVersion == 1 {
		// force HTTP1.1 as per https://pkg.go.dev/net/http#pkg-overview:
		// "Programs that must disable HTTP/2 can do so by setting
		//  Transport.TLSNextProto (for clients) or Server.TLSNextProto (for servers)
		//  to a non-nil, empty map."
		netTransport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	}

	// create the HTTP client
	j.client = &http.Client{
		// handle redirections here.
		// we count them, and reset metrics here
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			loc, err := req.Response.Location()
			if err != nil {
				j.Logger().Info().Msgf("redirected with error %v", err)
			} else {
				j.Logger().Info().Msgf("redirected to %s", loc.String())
			}
			j.wm.InitNew() //todo: evaluate
			j.numRedirect++
			j.finalTarget = loc.String()
			return nil // http.ErrUseLastResponse
		},
		Transport: rtTransport,
	}

	var body io.ReadCloser
	body = http.NoBody
	if j.Command == "POST" || j.Command == "PUT" {
		j.wm.ContentWriteSize = j.ContentSize
		j.wm.ContentReadSize = 0
		j.wm.WriteStepSize = 256 * 1024 // todo:
		j.wm.TotalWrite = 0
		body = j.wm
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

func (j *JobHTTPClient) Run(ctx context.Context, wc chan *Metric, wg *sync.WaitGroup) error {

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
			wm.Logger.Trace().Msgf("ClientTrace-DNSStart: %v", j.tr.start.Sub(refdate))
		},
		DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			j.tr.dnsDone = time.Now()
			j.tr.DNSInfo = dnsInfo
			wm.Logger.Trace().Msgf("ClientTrace-DNSDone: %v", j.tr.dnsDone.Sub(refdate))
		},
		GotConn: func(connInfo httptrace.GotConnInfo) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			j.tr.gotConn = time.Now()
			j.tr.ConnInfo = connInfo
			j.wm.Interface, j.wm.Gateway, j.wm.SourceAddr = utils.GetRoute(j.GetDestination())
			wm.Logger.Trace().Msgf("ClientTrace-GotConn: %v", j.tr.gotConn.Sub(refdate))
			wm.Logger.Debug().Msgf("GotConn - interface,gw,src: %s %s %s", wm.Interface.Name, wm.Gateway.String(), wm.SourceAddr.String())
		},
		GotFirstResponseByte: func() {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			j.tr.responseStart = time.Now()
			wm.FirstResponseByte = j.tr.responseStart // todo: does this belong here ?
			wm.Logger.Trace().Msgf("ClientTrace-GotFirstResponseByte: %v", j.tr.responseStart.Sub(refdate))
		},

		TLSHandshakeStart: func() {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			j.tr.tlsStart = time.Now()
			wm.Logger.Trace().Msgf("ClientTrace-TLSHandshakeStart: %v", j.tr.tlsStart.Sub(refdate))
		},

		TLSHandshakeDone: func(t tls.ConnectionState, err error) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			if err == nil {
				j.tr.tls = true
				j.tr.tlsDone = time.Now()
				j.tr.TLSDone = true
				j.tr.TLS = t
				wm.Logger.Trace().Msgf("ClientTrace-TLSHandshakeDone: %v (protocol = %s)", j.tr.tlsDone.Sub(refdate), t.NegotiatedProtocol)
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
			wm.Logger.Trace().Msgf("ClientTrace-onnectStart: %v", time.Since(refdate))
		},

		ConnectDone: func(net, addr string, err error) {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			j.tr.connectDone = time.Now()
			wm.Logger.Trace().Msgf("ClientTrace-ConnectDone: %v", j.tr.connectDone.Sub(refdate))
		},

		Got1xxResponse: func(code int, header textproto.MIMEHeader) error {
			wm.mutex.Lock()
			defer wm.mutex.Unlock()
			j.Logger().Trace().Msgf("ClientTrace-got 1xx: %d %+v", code, header)
			return nil
		},

		WroteHeaderField: func(key string, value []string) {
			j.Logger().Trace().Msgf("ClientTrace-WroteHeaderField %s = %v", key, value)
		},
		WroteHeaders: func() {
			j.Logger().Trace().Msgf("ClientTrace-WroteHeaders")
		},
		Wait100Continue: func() {
			j.Logger().Trace().Msgf("ClientTrace-Wait100Continue")
		},
		WroteRequest: func(wri httptrace.WroteRequestInfo) {
			j.wm.Logger.Trace().Err(wri.Err).Msgf("ClientTrace-WroteRequest")
			// todo: can we catch stuff here ?!
		},
	}

	// this is where we wait
	if j.Delay > 0 {
		j.Logger().Debug().Dur("delay", j.Delay).Msg("waiting")
		time.Sleep(j.Delay)
		j.Logger().Debug().Msg("waking up")
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

	utils.PrintMemUsage(j.Logger(), "before Do")
	resp, err := j.client.Do(j.req)
	utils.PrintMemUsage(j.Logger(), "after Do")
	j.tr.end = time.Now()

	err = fixH2H3Errors(err, j.req.Method, j.Logger())
	j.Logger().Debug().Msgf("client.Do: %v - %v", err, wm.Error)

	if errors.Is(err, context.DeadlineExceeded) {
		wm.Expired = true
		j.Logger().Info().Msgf("timer expired client side")
	} else if errors.Is(err, syscall.ECONNRESET) {
		wm.Expired = true
		j.Logger().Info().Msgf("something expired server side")
	} else if errors.Is(err, context.Canceled) {
		wm.Expired = true
		wm.Cancelled = true
		j.Logger().Info().Msgf("parent got cancelled")
	} else if err != nil {
		wm.Error = err
		j.Logger().Info().Err(err).Send()
		// todo: get body for more info ()
	}

	if j.tr.ConnInfo.Conn != nil {
		wm.Remote = j.tr.ConnInfo.Conn.RemoteAddr().String()
		wm.AvgLatency = j.tr.connectDone.Sub(j.tr.connectStart)
	}

	// completed or expired (timeout)
	if wm.Error == nil && !wm.Cancelled {
		if j.tr.ConnInfo.Conn != nil {
			j.Logger().Info().
				Stringer("LocalAddr", j.tr.ConnInfo.Conn.LocalAddr()).
				Stringer("RemoteAddr", j.tr.ConnInfo.Conn.RemoteAddr()).Send()
		}
		j.Logger().Info().
			Dur("DNSInfoTime", j.tr.dnsDone.Sub(refdate)). //wm.DNSInfoTime.Sub(refdate),
			Dur("ConnInfoTime", j.tr.gotConn.Sub(refdate)).
			Dur("FirstResponseByte", wm.FirstResponseByte.Sub(refdate)).
			Send()

		for _, a := range j.tr.DNSInfo.Addrs {
			j.Logger().Info().Stringer("dnsinfo", &a).Send()
		}

		if j.tr.ConnInfo.Reused {
			log.Info().Msg("REUSING CONNECTION")
		}

		if j.tr.TLSDone {
			j.Logger().Info().
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
		wm.Cipher = TLSCipherSuite(resp.TLS)
		wm.TLSVersion = utils.TLSVersionFriendlyName(TLSVersion(resp.TLS))

		j.Logger().Info().
			Str("protocol", wm.Protocol).
			Send()
		j.Logger().Debug().
			Int64("ContentLength", resp.ContentLength).
			Str("ContentLength-Decimal", utils.ByteCountDecimal(resp.ContentLength)+"B").
			Str("ContentLength-Binary", utils.ByteCountBinary(resp.ContentLength)+"B").
			Send()

		// dump resp if trace on
		if ShouldLog(j.Logger(), zerolog.TraceLevel) {
			dump, err := httputil.DumpResponse(resp, false)
			if err != nil {
				j.Logger().Fatal().Err(err).Send()
			}
			j.Logger().Debug().Msgf("Response:\n%s", dump)
		}

		if resp.StatusCode != http.StatusOK {
			wm.Error = errors.New("http status code: " + resp.Status)
			j.Logger().Debug().Msg(resp.Status)
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

				j.Logger().Info().Err(err).Msgf("copybuffer ended")

				err = fixH2H3Errors(err, j.req.Method, j.Logger())
				if errors.Is(err, context.DeadlineExceeded) {
					wm.Expired = true
					j.Logger().Info().Str("cause", "timer expired client side").Msg("ended")
				} else if err == io.ErrUnexpectedEOF {
					wm.Expired = true
					j.Logger().Info().Str("cause", "something expired server side").Msg("ended")
				} else if err != nil {
					j.Logger().Error().Err(err)
					wm.Valid = false
					wm.Error = err
				} else {
					j.Logger().Info().Str("cause", "completed normally").Msg("ended")
					if totalread != wm.TotalRead {
						j.Logger().Warn().Msgf("iocopy result differs from interface: %d - %d", totalread, wm.TotalRead)
					}
				}
			}
			// todo: get the server response for put/post here
			// if (j.Command == "PUT" || j.Command == "POST") && j.Logger().GetLevel() <= zerolog.TraceLevel {

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
			j.Logger().Debug().Msgf("closing Metric")
			wm.Close()
		}
	}
	wm.Done = true
	j.wm.Command = j.PrintString(false, false)

	utils.PrintMemUsage(j.Logger(), "client ended")
	wm.LogFinal(j.Logger())
	wc <- wm
	return nil
}

var errNotRunning = errors.New("named job not running")

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
			return "", NewNSError(errNotRunning, url.Host)
		}
		url.Scheme = s.URL.Scheme
		url.Host = s.URL.Host
		return url.String(), nil
	}
	return target, nil
}

// fixH2H3Errors tried to unify the behavior of HTTP/1, HTTP/2 and HTTP/3
func fixH2H3Errors(err error, method string, l *zerolog.Logger) error {
	// if err != nil {
	// 	fmt.Printf("error to change: %s %s\n", err, reflect.TypeOf(err))
	// }
	// HTTP/3 and ErrorCode == errorRequestCanceled
	var qerr *quic.StreamError
	if errors.As(err, &qerr) && qerr.ErrorCode == 0x10c {
		l.Warn().Msgf("quic.StreamError: %d changed to context.DeadlineExceeded ", qerr.ErrorCode)
		return context.DeadlineExceeded
	}

	// HTTP/2 & INTERNAL_ERROR & 'received from peer' => server timeout (WriteTimeout for instance)
	// this is not good, see: https://github.com/golang/go/issues/21389
	// for a PUT/POST timeout client side we also get INTERNAL_ERROR instead of syscall.ECONNRESET
	if h2err, ok := err.(http2.StreamError); ok {
		l.Debug().Msgf("h2 stream error %s", h2err)
		if h2err.Code == http2.ErrCodeInternal {
			if h2err.Cause != nil && h2err.Cause.Error() == "received from peer" {
				if method == "GET" {
					l.Warn().Msgf("h2 stream error changed to EOF")
					return io.ErrUnexpectedEOF
				}
				return syscall.ECONNRESET
			}
		}
		if h2err.Code == http2.ErrCodeStreamClosed {
			if h2err.Cause != nil && h2err.Cause.Error() == "received from peer" {
				l.Warn().Msgf("h2 stream closed changed to EOF")
				return io.ErrUnexpectedEOF
			}
		}
	}
	return err
}

/*

net/http errors returned in case of timeouts:
method:
   client.Do (c->s) then io.Read response body (s->c)
   we catch the err after the .Do and the .Read
   we set timer so the timeout happend during c->s
   then during s->c

client side timeouts:
caused by a http.Request with a context deadline

http/1.1:
	c->s: context deadline exceeded type: *url.Error
	s->c: context deadline exceeded type: context.deadlineExceededError
http/1.1 + tls:
	c->s: context deadline exceeded type: *url.Error
	s->c: context deadline exceeded type: context.deadlineExceededError
http/2:
	c->s: context deadline exceeded type: *url.Error
	s->c: context deadline exceeded type: context.deadlineExceededError
http/3:
	c->s: stream 0 canceled by local with error code 268 *url.Error
	s->c: stream 0 canceled by local with error code 268 *quic.StreamError

server side timeouts:

caused by a http.Server.WriteTimeout or ReadTimeout
or
http.NewResponseController.SetWriteDeadline/SetReadDeadline

http/1.1:
  c->s: no error !
  s->c: io.ErrUnexpectedEOF
http/1.1 + tls:
  c->s: "write: connection reset by peer"  type: *url.Error
  s->c: "tls: bad record MAC" type: *tls.permanentError
http/2:
  c->s: "stream error: stream ID 1; INTERNAL_ERROR; received from peer" type: *url.Error
  s->c: "stream ID 1; INTERNAL_ERROR; received from peer" type: http2.StreamError
http/3:
  WriteTimeout & ReadTimeout not handled
  SetWriteDeadline/SetReadDeadline not supported

*/
