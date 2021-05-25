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

func supportedTLS13Ciphers(hostname, port string) ([]uint16, error) {
	var supportedCiphers []uint16

	for _, c := range realDefaultCipherSuitesTLS13 {
		cfg := &tls.Config{
			ServerName: hostname,
			MinVersion: tls.VersionTLS13,
		}

		// Override the internal slice!
		varDefaultCipherSuitesTLS13 = []uint16{c}

		conn, err := net.Dial("tcp", net.JoinHostPort(hostname, port))
		if err != nil {
			return nil, err
		}

		client := tls.Client(conn, cfg)
		client.Handshake()
		client.Close()

		if client.ConnectionState().CipherSuite == c {
			supportedCiphers = append(supportedCiphers, c)
		}
	}

	// Reset the internal slice back to the full set
	varDefaultCipherSuitesTLS13 = realDefaultCipherSuitesTLS13

	return supportedCiphers, nil
}

func supportedTLS12Ciphers(hostname, port string) ([]uint16, error) {
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

		conn, err := net.Dial("tcp", net.JoinHostPort(hostname, port))
		if err != nil {
			return nil, err
		}

		client := tls.Client(conn, cfg)
		client.Handshake()
		client.Close()

		if client.ConnectionState().CipherSuite == c {
			supportedCiphers = append(supportedCiphers, c)
		}
	}

	return supportedCiphers, nil
}

// parse URL , happend default
func parseURL(u, scheme string) (*url.URL, error) {
	u1, err := url.ParseRequestURI(u)
	if err != nil || u1.Host == "" {
		u2, err := url.ParseRequestURI(scheme + "://" + u)
		if err != nil {
			return nil, err
		}
		return u2, nil
	}
	return u1, nil
}

//implement Job
type NSpeedCyphers struct {
	Target string
	Delay  uint
}

// print the cyphers list of hostname (dial address)
func (j *NSpeedCyphers) Start(ctx context.Context, wm *WorkerMetrics) error {

	u, _ := parseURL(j.Target, "https")

	//fmt.Println("url = ", u.Hostname())

	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "443"
	}

	if j.Delay > 0 {
		wm.Logger.Debug().Uint("delay", j.Delay).Msg("waiting")
		time.Sleep(time.Duration(j.Delay) * time.Second)
		wm.Logger.Debug().Msg("waking up")
	}

	range12, err := supportedTLS12Ciphers(host, port)
	if err != nil {
		return err
	}

	finalTarget := net.JoinHostPort(host, port)
	for _, c := range range12 {
		wm.Logger.Log().Msgf("%s supports TLS 1.2 %s", finalTarget, tls.CipherSuiteName(c))
	}

	range13, err := supportedTLS13Ciphers(host, port)
	if err != nil {
		return err
	}

	for _, c := range range13 {
		wm.Logger.Log().Msgf("%s supports TLS 1.3 %s", finalTarget, tls.CipherSuiteName(c))
	}

	wm.ResultChan <- wm

	return nil
}

func (j *NSpeedCyphers) GetTarget() string {
	return fmt.Sprintf("cyphers %s", j.Target)
}

func (j *NSpeedCyphers) Expand(jobs []Job) []Job {
	return append(jobs, j)
}

func (j *NSpeedCyphers) GoString() string {
	return fmt.Sprintf("cyphers {Target: %s, delay: %d}",
		j.Target,
		j.Delay,
	)
}

func (j *NSpeedCyphers) PrintString() string {
	return fmt.Sprintf("cyphers %s", j.Target)
}

func (j *NSpeedCyphers) IsInfinite() bool {
	return false
}
