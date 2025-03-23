package crypto

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// local certificate - ALWAYS Clone() it
var tlsSelfConfig *tls.Config

func init() {
	// self-signed cert & CA
	certificate, err := tls.X509KeyPair(TLSSelfSignedCert, TLSSelfSignedKey)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// https://forfuncsake.github.io/post/2017/08/trust-extra-ca-cert-in-go-app/
	// Get the SystemCertPool, continue with an empty pool on error
	rootCAs, _ := x509.SystemCertPool()
	if rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}
	// Append our cert to the system pool
	if ok := rootCAs.AppendCertsFromPEM(TLSSelfSignedCA); !ok {
		fmt.Println("error: couldn't append our embedded self-signed CA")
		os.Exit(1)
	}

	tlsSelfConfig = &tls.Config{
		Certificates: []tls.Certificate{certificate},
		RootCAs:      rootCAs,
		// CipherSuites: []uint16{tls.TLS_CHACHA20_POLY1305_SHA256},
		// MinVersion:   tls.VersionTLS13,
	}
}

func GetTLSSelfConfig() *tls.Config {
	return tlsSelfConfig.Clone()
}

// TLSCipherSuite return the CipherSuite name used or empty string if none
func TLSCipherSuite(cs *tls.ConnectionState) string {
	if cs == nil {
		return ""
	}
	return tls.CipherSuiteName(cs.CipherSuite)
}

// TLSCipherSuite return the CipherSuite name used or empty string if none
func TLSVersion(cs *tls.ConnectionState) uint16 {
	if cs == nil {
		return 0
	}
	return cs.Version
}
