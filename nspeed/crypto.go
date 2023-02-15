package nspeed

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

func init() {
	// self-signed cert & CA
	certificate, err := tls.X509KeyPair(TLSSelfSignedCert, TLSSelfSignedKey)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// todo: move this in a func and Once it or 'init' it
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

	TLSSelfConfig = &tls.Config{
		Certificates: []tls.Certificate{certificate},
		RootCAs:      rootCAs,
		// CipherSuites: []uint16{tls.TLS_CHACHA20_POLY1305_SHA256},
		// MinVersion:   tls.VersionTLS13,
	}
}
