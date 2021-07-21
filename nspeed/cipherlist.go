package nspeed

// mainly a copy/paste of https://github.com/joeshaw/cipherlist

// Using go:linkname requires us to import unsafe
import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"time"
	_ "unsafe"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// from:

// We bring the real defaultCipherSuitesTLS13 function from the
// crypto/tls package into our own package.  This lets us perform
// that lazy initialization of the cipher list when we want.

//go:linkname defaultCipherSuitesTLS13 crypto/tls.defaultCipherSuitesTLS13
func defaultCipherSuitesTLS13() []uint16

// Next we bring the `varDefaultCipherSuitesTLS13` slice into our
// package.  This is what we manipulate to get the cipher suites.

//go:linkname varDefaultCipherSuitesTLS13 crypto/tls.varDefaultCipherSuitesTLS13
var varDefaultCipherSuitesTLS13 []uint16

// Also keep a variable around for the real default set, so we
// can reset it once we're finished.
var realDefaultCipherSuitesTLS13 []uint16

func init() {
	// Initialize the TLS 1.3 ciphersuite set; this populates
	// varDefaultCipherSuitesTLS13 under the covers
	realDefaultCipherSuitesTLS13 = defaultCipherSuitesTLS13()
}

func supportedTLS13Ciphers(hostname, port string, timeout time.Duration) ([]uint16, error) {
	var supportedCiphers []uint16

	for _, c := range realDefaultCipherSuitesTLS13 {
		cfg := &tls.Config{
			ServerName: hostname,
			MinVersion: tls.VersionTLS13,
		}

		// Override the internal slice!
		varDefaultCipherSuitesTLS13 = []uint16{c}

		conn, err := net.DialTimeout("tcp", net.JoinHostPort(hostname, port), timeout)
		if err != nil {
			return nil, err
		}

		client := tls.Client(conn, cfg)
		err = client.Handshake()
		client.Close()

		if err != nil {
			return nil, err
		}

		if client.ConnectionState().CipherSuite == c {
			supportedCiphers = append(supportedCiphers, c)
		}
	}

	// Reset the internal slice back to the full set
	varDefaultCipherSuitesTLS13 = realDefaultCipherSuitesTLS13

	return supportedCiphers, nil
}

func supportedTLS12Ciphers(hostname, port string, timeout time.Duration) ([]uint16, error) {
	// Taken from https://golang.org/pkg/crypto/tls/#pkg-constants
	var allCiphers = []uint16{
		tls.TLS_RSA_WITH_RC4_128_SHA,
		tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
		tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
	}

	var supportedCiphers []uint16

	for _, c := range allCiphers {
		cfg := &tls.Config{
			ServerName:   hostname,
			CipherSuites: []uint16{c},
			MinVersion:   tls.VersionTLS12,
			MaxVersion:   tls.VersionTLS12,
		}

		log.Debug().Msgf("dialing ...%s:%s (%s)", hostname, port, timeout.String())
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(hostname, port), timeout)
		if err != nil {
			return nil, err
		}

		client := tls.Client(conn, cfg)
		err = client.Handshake()
		client.Close()
		if err != nil {
			return nil, err
		}

		if client.ConnectionState().CipherSuite == c {
			supportedCiphers = append(supportedCiphers, c)
		}
	}

	return supportedCiphers, nil
}

// parse URL , append default port
func parseURL(u, scheme string) (*url.URL, error) {
	u1, err := url.ParseRequestURI(u)
	if err != nil || u1.Host == "" {
		//fmt.Println("error 1")
		u2, err := url.ParseRequestURI(scheme + "://" + u)
		if err != nil {
			//fmt.Println("error 2")
			return nil, err
		}
		return u2, nil
	}
	return u1, nil
}

//implement Job
type JobCiphers struct {
	Target  string
	Delay   uint
	host    string
	port    string
	Timeout time.Duration
	wm      *Metrics
}

func (j *JobCiphers) Init(wm *Metrics) error {

	u, err := parseURL(j.Target, "https")
	fmt.Println("err = ", err)
	fmt.Println("url = ", u.Hostname())
	j.host = u.Hostname()
	j.port = u.Port()
	if j.port == "" {
		j.port = "443"
	}

	if j.Timeout == 0 {
		j.Timeout = 3 * time.Second
	}

	j.wm = wm

	return nil
}

// print the ciphers list of hostname (dial address)
func (j *JobCiphers) Run(ctx context.Context) error {

	if j.Delay > 0 {
		j.wm.Logger.Debug().Uint("delay", j.Delay).Msg("waiting")
		time.Sleep(time.Duration(j.Delay) * time.Second)
		j.wm.Logger.Debug().Msg("waking up")
	}

	range12, err := supportedTLS12Ciphers(j.host, j.port, j.Timeout)
	if err != nil {
		return err
	}
	LogCipher(j.host, j.port, TLSVersionFriendlyName(tls.VersionTLS12), range12, j.wm.Logger)

	range13, err := supportedTLS13Ciphers(j.host, j.port, j.Timeout)
	if err != nil {
		return err
	}
	LogCipher(j.host, j.port, TLSVersionFriendlyName(tls.VersionTLS13), range13, j.wm.Logger)

	j.wm.ResultChan <- j.wm

	return nil
}

func (j *JobCiphers) GetTarget() string {
	return fmt.Sprintf("ciphers %s", j.Target)
}

func (j *JobCiphers) Expand(jobs []Job, group int) []Job {
	return append(jobs, j)
}

func (j *JobCiphers) GoString() string {
	return fmt.Sprintf("ciphers {Target: %s (%s;%s), delay: %d, timeout:%s}",
		j.Target,
		j.host, j.port,
		j.Delay,
		j.Timeout,
	)
}

func (j *JobCiphers) PrintString() string {
	return fmt.Sprintf("ciphers %s (%s:%s)", j.Target, j.host, j.port)
}

func (j *JobCiphers) IsInfinite() bool {
	return false
}

// print out the cipher list and target using a Log
func LogCipher(host, port, tlsVersion string, ciphers []uint16, log zerolog.Logger) {
	finalTarget := net.JoinHostPort(host, port)
	for _, c := range ciphers {
		log.Info().Msgf("%s supports %s %s", finalTarget, tlsVersion, tls.CipherSuiteName(c))
		fmt.Printf("%s supports %s %s", finalTarget, tlsVersion, tls.CipherSuiteName(c))
	}
	if len(ciphers) == 0 {
		log.Info().Msgf("%s doesn't support %s", finalTarget, tlsVersion)
		fmt.Printf("%s doesn't support %s", finalTarget, tlsVersion)
	}
}
