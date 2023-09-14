package nspeed

import "sort"

// banchmark commands, same as nspeed cmd line arguments
// in Go template format
type Benchmark struct {
	Tpl  string
	Name string
}

type BenchmarkArgs struct {
	Args string
	N    int
	Size string
	ID   int
}

var benchmarksTemplates = map[string]Benchmark{
	// TCP
	"tp": {
		Tpl:  "server {{.Args}} -id s{{.ID}} -P tcp put -P tcp {{.Args}} -n {{.N}} nspeed://s{{.ID}}/ {{.Size}}",
		Name: "TCP PUT, localhost",
	},
	"tsp": {
		Tpl:  "server {{.Args}} -id s{{.ID}} -P tcp -self put -self -P tcp {{.Args}} -n {{.N}} nspeed://s{{.ID}}/ {{.Size}}",
		Name: "TCP PUT, localhost, tls",
	},
	// HTTP/1.1
	"h1g": {
		Tpl:  "server {{.Args}} -id s{{.ID}} get {{.Args}} -n {{.N}} nspeed://s{{.ID}}/{{.Size}}",
		Name: "HTTP/1.1 GET, localhost",
	},
	"h1p": {
		Tpl:  "server {{.Args}} -id s{{.ID}} put {{.Args}} -n {{.N}} nspeed://s{{.ID}}/ {{.Size}}",
		Name: "HTTP/1.1 PUT, localhost",
	},

	// HTTP/2C
	"h2cg": {
		Tpl:  "server -h2c {{.Args}} -id s{{.ID}} get {{.Args}} -n {{.N}} -h2c nspeed://s{{.ID}}/{{.Size}}",
		Name: "HTTP/2 GET, localhost, H2C (no encryption)",
	},
	"h2cp": {
		Tpl:  "server -h2c {{.Args}} -id s{{.ID}} put {{.Args}} -n {{.N}} -h2c nspeed://s{{.ID}}/ {{.Size}}",
		Name: "HTTP/2 PUT, localhost, h2c (no encryption)",
	},

	// HTTPS/1.1
	"h1sg": {
		Tpl:  "server {{.Args}} -id s{{.ID}} -self get {{.Args}} -n {{.N}} -self -http1.1 nspeed://s{{.ID}}/{{.Size}}",
		Name: "HTTPS/1.1 GET, localhost, TLS",
	},
	"h1sp": {
		Tpl:  "server {{.Args}} -id s{{.ID}} -self put {{.Args}} -n {{.N}} -self -http1.1 nspeed://s{{.ID}}/ {{.Size}}",
		Name: "HTTPS/1.1 GET, localhost, TLS",
	},

	// HTTPS/2
	"h2g": {
		Tpl:  "server {{.Args}} -id s{{.ID}} -self get {{.Args}} -n {{.N}} -self -http2 nspeed://s{{.ID}}/{{.Size}}",
		Name: "HTTPS/2 GET , localhost, tls",
	},
	"h2p": {
		Tpl:  "server {{.Args}} -id s{{.ID}} -self put {{.Args}} -n {{.N}} -self -http2 nspeed://s{{.ID}}/ {{.Size}}",
		Name: "HTTPS/2 PUT , localhost, tls",
	},

	// HTTPS/3
	"h3g": {
		Tpl:  "server {{.Args}} -id s{{.ID}} -http3 get {{.Args}} -n {{.N}} -self -http3 nspeed://s{{.ID}}/{{.Size}}",
		Name: "HTTP/3 GET, localhost, tls, udp",
	},
	"h3p": {
		Tpl:  "server {{.Args}} -id s{{.ID}} -http3 put {{.Args}} -n {{.N}} -self -http3 nspeed://s{{.ID}}/ {{.Size}}",
		Name: "HTTP/3 PUT, localhost, tls, udp",
	},
}

// sorted keys of benchmarksTemplates
var benchmarKeys []string

// sort the keys
func init() {
	benchmarKeys = make([]string, 0, len(benchmarksTemplates))

	for k := range benchmarksTemplates {
		benchmarKeys = append(benchmarKeys, k)
	}
	sort.Strings(benchmarKeys)
}
