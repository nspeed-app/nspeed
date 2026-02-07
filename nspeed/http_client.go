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
	"net/netip"
	"net/textproto"
	"net/url"
	"nspeed/metrics"
	"slices"
	"sync/atomic"
	"syscall"
	"time"

	quicgo "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/rs/zerolog"
	"golang.org/x/net/http2"
	"nspeed.app/crypto"
	"nspeed.app/humanize"
	"nspeed.app/iobuffer"
	"nspeed.app/network"
	"nspeed.app/pacing"
)

// HTTP specific client code - complement of client.go

// this will be used to set a big value for MaxReadFrameSize (HTTP/2)
// see https://github.com/golang/go/issues/54850
const h2MaxFrameSize uint32 = 1 << 20 // 1024 * 1024

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

func (j *JobClient) HTTPInit() error {

	// we clone to avoid ConfigureTransports to crash
	// var netTransport = http.DefaultTransport.(*http.Transport).Clone()
	var netTransport = &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     j.HTTPVersion == 2, // what about -http3?
		MaxIdleConns:          100,
		IdleConnTimeout:       10 * time.Second, // add parameter for this ?
		TLSHandshakeTimeout:   5 * time.Second,  // this seems long ?
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    true,
		//  transforms network "tcp" / "udp" to "tcp4"/"udp4" or "tcp6"/"udp6" if IPVerson is set.
		DialContext:     j.dialContext,
		TLSClientConfig: j.tlsConfig,
	}

	// interface to the transport so we can switch if needed
	var rtTransport http.RoundTripper = netTransport

	// HTTP/2
	if j.HTTPVersion == 2 {
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
		idx := slices.Index(netTransport.TLSClientConfig.NextProtos, "http/1.1")
		if idx != -1 {
			j.Logger().Debug().Msgf("removing http/1.1 from netTransport.TLSClientConfig")
			netTransport.TLSClientConfig.NextProtos = append(netTransport.TLSClientConfig.NextProtos[:idx], netTransport.TLSClientConfig.NextProtos[idx+1:]...)
		}
	}
	// unencryped (ClearText) HTTP/2
	if j.HTTPVersion == 2 && j.H2C {
		rtTransport = &http2.Transport{
			DialTLSContext: func(ctx context.Context, network string, addr string, cfg *tls.Config) (net.Conn, error) {
				return j.dialContext(ctx, network, addr)
			},
			TLSClientConfig:    netTransport.TLSClientConfig.Clone(),
			DisableCompression: true,
			AllowHTTP:          j.H2C,
			MaxReadFrameSize:   h2MaxFrameSize,
		}
	}

	// HTTP/3
	if j.HTTPVersion == 3 {
		rtTransport, j.closer = NewQUICGOHTTP3Transport(netTransport, j)
	}

	// HTTP/1.x
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
	if j.Command == "GET" {
		j.wm.ContentWriteSize = 0
		j.wm.ContentReadSize = j.ContentSize * int64(j.Repeat)
		j.wm.TotalRead = 0
	}
	req, err := http.NewRequest(j.Command, j.finalTarget, body)
	if err != nil {
		_ = j.Close() // not good, find a better way to clean opened port
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

func (j *JobClient) HTTPRun(ctx context.Context, _ Options) (*metrics.Metric, error) {

	if j.req == nil {
		return nil, errors.New("no request")
	}

	if ctx != nil {
		j.req = j.req.WithContext(ctx)
	}

	wm := j.wm
	// tracing & timing stuff
	refdate := time.Now()

	var connected atomic.Bool
	// todo handle redirects more nicely (array ?)
	trace := &httptrace.ClientTrace{
		DNSStart: func(dnsInfo httptrace.DNSStartInfo) {
			j.tr.start = time.Now()
			wm.Logger().Trace().Msgf("ClientTrace-DNSStart: %v", j.tr.start.Sub(refdate))
		},
		DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
			j.tr.dnsDone = time.Now()
			j.tr.DNSInfo = dnsInfo
			wm.Logger().Trace().Msgf("ClientTrace-DNSDone: %v", j.tr.dnsDone.Sub(refdate))
		},
		GotConn: func(connInfo httptrace.GotConnInfo) {
			connected.Store(true)
			j.tr.gotConn = time.Now()
			j.tr.ConnInfo = connInfo
			var err error
			j.wm.SourceAddr, err = netip.ParseAddrPort(connInfo.Conn.LocalAddr().String())
			if err != nil {
				wm.Logger().Debug().Err(err).Msgf("error parsing local address:%s", connInfo.Conn.LocalAddr())
			}
			j.wm.Interface, j.wm.Gateway, j.wm.RouteSourceAddr, err = network.GetRouteFromAddrPort(j.GetDestination())
			if err != nil {
				wm.Logger().Error().Err(err).Msgf("ClientTrace-GotConn")
				return
			}
			wm.Logger().Trace().Msgf("ClientTrace-GotConn: %v", j.tr.gotConn.Sub(refdate))
			wm.Logger().Debug().Msgf("GotConn - interface,gw,src: %s %s %s", wm.Interface.Name, wm.Gateway.String(), wm.SourceAddr.String())
		},
		GotFirstResponseByte: func() {
			j.tr.responseStart = time.Now()
			wm.FirstResponseByte = j.tr.responseStart // todo: does this belong here ?
			wm.Logger().Trace().Msgf("ClientTrace-GotFirstResponseByte: %v", j.tr.responseStart.Sub(refdate))
		},

		TLSHandshakeStart: func() {
			j.tr.tlsStart = time.Now()
			wm.Logger().Trace().Msgf("ClientTrace-TLSHandshakeStart: %v", j.tr.tlsStart.Sub(refdate))
		},

		TLSHandshakeDone: func(t tls.ConnectionState, err error) {
			if err == nil {
				j.tr.tls = true
				j.tr.tlsDone = time.Now()
				j.tr.TLSDone = true
				j.tr.TLS = t
				wm.Logger().Trace().Msgf("ClientTrace-TLSHandshakeDone: %v (protocol = %s)", j.tr.tlsDone.Sub(refdate), t.NegotiatedProtocol)
			} else {
				j.tr.tls = false
				//fmt.Printf("tls: %+v \n", err)
			}
		},
		ConnectStart: func(_, _ string) {
			// No DNS resolution means we used an IP address directly.
			j.tr.connectStart = time.Now()
			if j.tr.dnsDone.IsZero() {
				j.tr.start = j.tr.connectStart
				j.tr.dnsDone = j.tr.start
			}
			wm.Logger().Trace().Msgf("ClientTrace-ConnectStart: %v", time.Since(refdate))
		},

		ConnectDone: func(net, addr string, err error) {
			j.tr.connectDone = time.Now()
			wm.Logger().Trace().Err(err).Msgf("ClientTrace-ConnectDone: %v", j.tr.connectDone.Sub(refdate))
		},

		Got1xxResponse: func(code int, header textproto.MIMEHeader) error {
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
			j.wm.Logger().Trace().Err(wri.Err).Msgf("ClientTrace-WroteRequest")
			// todo: can we catch stuff here ?!
		},
		PutIdleConn: func(err error) {
			j.wm.Logger().Trace().Err(err).Msgf("ClientTrace-PutIdleConn")
		},
	}

	// this is where we wait -fixme: use select / context to catch interupt
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
	if j.Repeat == 0 {
		j.Logger().Warn().Msgf("zero repeat!") //should this fail?
	}
	wm.LoopMax = j.Repeat
	wm.Loop = 0
	var totalwrite int64 = 0
	// main loop for -r
	for loop := range j.Repeat {
		wm.Loop += 1
		LogMemUsage(j.Logger(), "before Loop #%d", loop)
		done := make(chan any)
		if j.Command == "PUT" || j.Command == "POST" {
			wm.StartTicker(done)
		}
		// ***** http request happens here
		if (j.Command == "PUT" || j.Command == "POST") && j.Schedule != nil {
			j.req.Body = io.NopCloser(pacing.NewPacedReader(j.req.Context(), j.req.Body, j.Schedule))
		}
		resp, err := j.client.Do(j.req)
		// *****
		if j.Command == "PUT" || j.Command == "POST" {
			close(done)
		}
		// fmt.Printf("LOOP: ")
		// wm.PrintFinal(os.Stdout)
		LogMemUsage(j.Logger(), "after Do loop #%d", loop)
		j.tr.end = time.Now()
		err = fixH2H3Errors(err, j.req.Method, j.Logger())
		j.Logger().Info().Msgf("client.Do (loop %d): %v - %v", loop, err, wm.Error)

		// wm.Error can be filled by handler: TLSHandshakeDone and ConnectDone
		if errors.Is(err, context.DeadlineExceeded) {
			wm.Expired = true
			j.Logger().Info().Msgf("timer expired client side")
			// timeout without a gotConn, it's an error
			if !connected.Load() {
				wm.Error = err
			}
		} else if errors.Is(err, syscall.ECONNRESET) {
			wm.Expired = true
			j.Logger().Info().Msgf("something expired server side")
		} else if errors.Is(err, context.Canceled) {
			wm.Expired = true
			wm.Cancelled = true
			j.Logger().Info().Msgf("parent got cancelled")
		} else if err != nil {
			wm.Error = err
			j.Logger().Info().Err(err).Send() // TODO FIX
			// todo: get body for more info ()
		}

		if j.tr.ConnInfo.Conn != nil {
			wm.Remote, err = netip.ParseAddrPort(j.tr.ConnInfo.Conn.RemoteAddr().String())
			if err != nil {
				j.Logger().Debug().Err(err).Msgf("error parsing remote address: %s", j.tr.ConnInfo.Conn.RemoteAddr().String())
			}
			wm.AvgLatency = j.tr.connectDone.Sub(j.tr.connectStart)
		}

		// completed or expired (timeout)
		if wm.Error == nil && !wm.Cancelled {
			if j.tr.ConnInfo.Conn != nil {
				j.Logger().Debug().
					Stringer("LocalAddr", j.tr.ConnInfo.Conn.LocalAddr()).
					Stringer("RemoteAddr", j.tr.ConnInfo.Conn.RemoteAddr()).Send()
			}
			j.Logger().Debug().
				Dur("DNSInfoTime", j.tr.dnsDone.Sub(refdate)). //wm.DNSInfoTime.Sub(refdate),
				Dur("ConnInfoTime", j.tr.gotConn.Sub(refdate)).
				Dur("FirstResponseByte", wm.FirstResponseByte.Sub(refdate)).
				Send()

			for _, a := range j.tr.DNSInfo.Addrs {
				j.Logger().Debug().Stringer("dnsinfo", &a).Send()
			}

			if !j.tr.ConnInfo.Reused && loop > 1 {
				j.Logger().Warn().Msg("Repeat not reusing the connection") // should we abort ? at least set this somewhere in wm
			}

			if j.tr.TLSDone {
				j.Logger().Debug().
					Str("crypto", crypto.TLSVersionName(j.tr.TLS.Version)).
					Str("cipher", tls.CipherSuiteName(j.tr.TLS.CipherSuite)).
					Dur("TLSDuration", j.tr.tlsDone.Sub(refdate)).Send()
			}
		}
		// todo: time'd out 'POST/PUT' don't have a resp so we're missing informations like protocol ,tls, etc
		// todo: find a way to get the missing informations elsewhere ?
		// possible workaround: do a HEAD before the POST/PUT but that will count has 2 requests and could mess the server stats and Maxrequests option
		// or HEAD shouldn't count as a request server side ?
		if wm.Error == nil && resp != nil {
			wm.Protocol = resp.Proto
			wm.Cipher = crypto.TLSCipherSuite(resp.TLS)
			wm.TLSVersion = crypto.TLSVersionFriendlyName(crypto.TLSVersion(resp.TLS))

			j.Logger().Debug().
				Str("protocol", wm.Protocol).
				Send()
			j.Logger().Debug().
				Int64("ContentLength", resp.ContentLength).
				Str("ContentLength-Decimal", humanize.ByteCountDecimal(resp.ContentLength)+"B").
				Str("ContentLength-Binary", humanize.ByteCountBinary(resp.ContentLength)+"B").
				Send()

			// dump resp if trace on
			if ShouldLog(j.Logger(), zerolog.TraceLevel) {
				dump, err := httputil.DumpResponse(resp, false)
				if err != nil {
					j.Logger().Fatal().Err(err).Send()
				}
				j.Logger().Trace().Msgf("Response:\n%s", dump)
			}

			if resp.StatusCode != http.StatusOK {
				wm.Error = errors.New("http status code: " + resp.Status)
				j.Logger().Debug().Msg(resp.Status)
				wm.Valid = false
			} else { // here we consume the body
				if j.Command == "GET" {
					defer func() {
						if resp.Body != nil {
							_ = resp.Body.Close()
						}
					}()
					wm.ContentReadSize += resp.ContentLength
					// if wm.ContentReadSize == 0 && resp.ContentLength > 0 {
					// 	wm.ContentReadSize = resp.ContentLength
					// }
					wm.StartTicker(done)
					// this is where the GET transfert happens.
					//   iteration 3: we use io.CopyBuffer the body to 'wm' (which is a Discard) and
					//   the resp (thru req) context will handle timeout & parent cancellation

					//wm.TotalReadProcessed, err = io.Copy(ioutil.Discard, io.TeeReader(resp.Body, wm))
					//todo: invesstigate why the following gives results too high

					var bodyReader io.Reader = resp.Body
					if j.Schedule != nil {
						bodyReader = pacing.NewPacedReader(ctx, resp.Body, j.Schedule)
					}

					totalread, err := iobuffer.Copy(wm, bodyReader)

					// b, err := io.ReadAll(resp.Body)
					// if err != nil {
					// 	log.Error().Err(err).Msg("read response.Body")
					// }
					// totalread := len(b)
					// if totalread == 0 {
					// 	log.Error().Msg("empty body")
					// } else {
					// 	fmt.Println("HEAD OF BOBY - size = ", totalread)
					// 	fmt.Println(string(b[:min(1000, len(b)-1)]))
					// 	fmt.Println("----------------------------------------------------------------------------")
					// }

					close(done)
					j.Logger().Debug().Err(err).Msgf("copybuffer ended with %d totalread", totalread)

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
						// disable this because of loops (-r) - todo: refactor the whole loop
						// if totalread != wm.TotalRead {
						// 	j.Logger().Info().Msgf("less data was read: %d - %d", totalread, wm.TotalRead)
						// }
					}
				}
				// todo: get the server response for put/post here
				// if (j.Command == "PUT" || j.Command == "POST") && j.Logger().GetLevel() <= zerolog.TraceLevel {

				// if loop == j.Repeat-1 {
				// 	j.Logger().Debug().Msgf("closing Metric")
				// }
			}
		}
		// this is bad: refactor the 'loop' stuff to store each loop metrics
		_ = wm.Close() // todo check if loop
		totalwrite += j.wm.TotalWrite
		j.wm.TotalWrite = 0
		// conditions to stop Repeat
		if wm.Error != nil || wm.Expired || !wm.Valid {
			break // Repeat
		}
	} // Repeat
	j.wm.TotalWrite = int64(totalwrite)
	// fmt.Printf("POST: ")
	// wm.PrintFinal(os.Stdout)
	j.client.CloseIdleConnections()
	wm.Done = true
	j.wm.Command = j.PrintString(false, false)

	LogMemUsage(j.Logger(), "client ended")
	wm.LogFinal(j.Logger())
	return wm, nil
}

var errNotRunning = errors.New("named job not running")

// ResolveSchemes handles special schemes:
//   - "nspeed://id" will return the URL of the job with matching id
func ResolveSchemes(idmap map[string]Job, target string) (*url.URL, error) {

	u, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "nspeed" {
		job, ok := idmap[u.Host]
		if !ok {
			return nil, fmt.Errorf("named job '%s' not found", u.Host)
		}
		// todo: not a fan of this. it's too dynamic
		// use a named interface instead ?
		if s, sok := job.(interface{ GetURL() *url.URL }); sok {
			if s.GetURL() == nil {
				return nil, NewNSError(errNotRunning, u.Host)
			}
			u.Scheme = s.GetURL().Scheme
			u.Host = s.GetURL().Host
			return u, nil
		}
		return nil, fmt.Errorf("named job '%s' is not a server", u.Host)
	}
	return u, nil
}

// fixH2H3Errors tried to unify the behavior of HTTP/1, HTTP/2 and HTTP/3
func fixH2H3Errors(err error, method string, l *zerolog.Logger) error {
	if err == nil {
		return err
	}

	// if err != nil {
	// 	fmt.Printf("error to change: %s %s\n", err, reflect.TypeOf(err))
	// }
	// HTTP/3 and ErrorCode == errorRequestCanceled
	var qerr *quicgo.StreamError
	if errors.As(err, &qerr) && http3.ErrCode(qerr.ErrorCode) == http3.ErrCodeRequestCanceled {
		l.Info().Msgf("quic.StreamError: %d changed to context.DeadlineExceeded ", qerr.ErrorCode)
		return context.DeadlineExceeded
	}

	var h3err *http3.Error
	if errors.As(err, &h3err) && h3err.ErrorCode == http3.ErrCodeRequestCanceled {
		l.Info().Msgf("http3.Error: %s changed to context.DeadlineExceeded ", h3err.ErrorCode.String())
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
					l.Info().Msgf("h2 stream error changed to EOF")
					return io.ErrUnexpectedEOF
				}
				return syscall.ECONNRESET
			}
		}
		if h2err.Code == http2.ErrCodeStreamClosed {
			if h2err.Cause != nil && h2err.Cause.Error() == "received from peer" {
				l.Info().Msgf("h2 stream closed changed to EOF")
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
