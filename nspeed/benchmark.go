package nspeed

import (
	"sort"
)

// banchmark commands, same as nspeed cmd line arguments
// in Go template format
type Benchmark struct {
	Tpl  string
	Name string
	Help string
}

type BenchmarkArgs struct {
	SArgs string
	CArgs string
	N     int
	Size  string
	ID    int
}

var benchmarksTemplates = map[string]Benchmark{
	// QUIC (stdlib implementation)
	"qp": {
		Tpl:  "server {{.SArgs}} -id s{{.ID}} -P quic put -self -P quic {{.CArgs}} -n {{.N}} nspeed://s{{.ID}}/ {{.Size}}",
		Name: "QUIC PUT",
		Help: "QUIC PUT, localhost",
	},
	"qg": {
		Tpl:  "server {{.SArgs}} -id s{{.ID}} -P quic get -size {{.Size}} -self -P quic {{.CArgs}} -n {{.N}} nspeed://s{{.ID}}/",
		Name: "QUIC GET",
		Help: "QUIC GET, localhost",
	},
	// QUIC (quic-go implementation)
	"qgp": {
		Tpl:  "server {{.SArgs}} -id s{{.ID}} -P quic-go put -self -P quic-go {{.CArgs}} -n {{.N}} nspeed://s{{.ID}}/ {{.Size}}",
		Name: "QUIC-GO PUT",
		Help: "QUIC-GO PUT, localhost",
	},
	"qgg": {
		Tpl:  "server {{.SArgs}} -id s{{.ID}} -P quic-go get -self -size {{.Size}} -P quic-go {{.CArgs}} -n {{.N}} nspeed://s{{.ID}}/",
		Name: "QUIC-GO GET",
		Help: "QUIC-GO GET, localhost",
	},
	// TCP
	"tp": {
		Tpl:  "server {{.SArgs}} -id s{{.ID}} -P tcp put -P tcp {{.CArgs}} -n {{.N}} nspeed://s{{.ID}}/ {{.Size}}",
		Name: "TCP PUT",
		Help: "TCP PUT, localhost",
	},
	"tsp": {
		Tpl:  "server {{.SArgs}} -id s{{.ID}} -P tcp -self put -self -P tcp {{.CArgs}} -n {{.N}} nspeed://s{{.ID}}/ {{.Size}}",
		Name: "TCP PUT TLS",
		Help: "TCP PUT, localhost, tls",
	},
	"tg": {
		Tpl:  "server {{.SArgs}} -id s{{.ID}} -P tcp get -size {{.Size}} -P tcp {{.CArgs}} -n {{.N}} nspeed://s{{.ID}}/",
		Name: "TCP GET",
		Help: "TCP GET, localhost",
	},
	"tsg": {
		Tpl:  "server {{.SArgs}} -id s{{.ID}} -P tcp -self get -self -size {{.Size}} -P tcp {{.CArgs}} -n {{.N}} nspeed://s{{.ID}}/",
		Name: "TCP GET TLS",
		Help: "TCP GET, localhost, tls",
	},
	// HTTP/1.1
	"h1cg": {
		Tpl:  "server {{.SArgs}} -id s{{.ID}} get {{.CArgs}} -n {{.N}} nspeed://s{{.ID}}/{{.Size}}",
		Name: "HTTP/1 GET",
		Help: "HTTP/1.1 GET, localhost",
	},
	"h1cp": {
		Tpl:  "server {{.SArgs}} -id s{{.ID}} put {{.CArgs}} -n {{.N}} nspeed://s{{.ID}}/ {{.Size}}",
		Name: "HTTP/1 PUT",
		Help: "HTTP/1.1 PUT, localhost",
	},

	// HTTP/2C
	"h2cg": {
		Tpl:  "server -h2c {{.CArgs}} -id s{{.ID}} get {{.CArgs}} -n {{.N}} -h2c nspeed://s{{.ID}}/{{.Size}}",
		Name: "HTTP/2 GET",
		Help: "HTTP/2 GET, localhost, H2C (no encryption)",
	},
	"h2cp": {
		Tpl:  "server -h2c {{.CArgs}} -id s{{.ID}} put {{.CArgs}} -n {{.N}} -h2c nspeed://s{{.ID}}/ {{.Size}}",
		Name: "HTTP/2 PUT",
		Help: "HTTP/2 PUT, localhost, h2c (no encryption)",
	},

	// HTTPS/1.1
	"h1g": {
		Tpl:  "server {{.SArgs}} -http1.1 -id s{{.ID}} -self get {{.CArgs}} -n {{.N}} -self -http1.1 nspeed://s{{.ID}}/{{.Size}}",
		Name: "HTTPS/1.1 GET",
		Help: "HTTPS/1.1 GET, localhost, TLS",
	},
	"h1p": {
		Tpl:  "server {{.SArgs}} -http1.1 -id s{{.ID}} -self put {{.CArgs}} -n {{.N}} -self -http1.1 nspeed://s{{.ID}}/ {{.Size}}",
		Name: "HTTPS/1.1 PUT",
		Help: "HTTPS/1.1 PUT, localhost, TLS",
	},

	// HTTPS/2
	"h2g": {
		Tpl:  "server {{.SArgs}} -http2 -id s{{.ID}} -self get {{.CArgs}} -n {{.N}} -self -http2 nspeed://s{{.ID}}/{{.Size}}",
		Name: "HTTPS/2 GET",
		Help: "HTTPS/2 GET , localhost, tls",
	},
	"h2p": {
		Tpl:  "server {{.SArgs}} -http2 -id s{{.ID}} -self put {{.CArgs}} -n {{.N}} -self -http2 nspeed://s{{.ID}}/ {{.Size}}",
		Name: "HTTPS/2 PUT",
		Help: "HTTPS/2 PUT , localhost, tls",
	},

	// HTTPS/3
	"h3g": {
		Tpl:  "server {{.SArgs}} -id s{{.ID}} -http3 get -self {{.CArgs}} -n {{.N}} -http3 nspeed://s{{.ID}}/{{.Size}}",
		Name: "HTTPS/3 GET",
		Help: "HTTP/3 GET, localhost, tls, udp",
	},
	"h3p": {
		Tpl:  "server {{.SArgs}} -id s{{.ID}} -http3 put -self {{.CArgs}} -n {{.N}} -http3 nspeed://s{{.ID}}/ {{.Size}}",
		Name: "HTTPS/3 PUT",
		Help: "HTTP/3 PUT, localhost, tls, udp",
	},
}

// sorted keys of benchmarksTemplates
var benchKeys []string

// sort the keys
func init() {
	benchKeys = make([]string, 0, len(benchmarksTemplates))

	for k := range benchmarksTemplates {
		benchKeys = append(benchKeys, k)
	}
	sort.Strings(benchKeys)
}
