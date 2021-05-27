package nspeed

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"
)

// version
var Version string

// commands
const getCommand = "get"
const putCommand = "put"
const serveCommand = "server"
const ciphersCommand = "ciphers"

// supported commands, expand has needed
type NSpeedCommand struct {
	Name string
	Set  int
}

// list of supported commands. Value is the set number. Commands in the same set can be mixed together.
var ImplementedCommands = map[string]NSpeedCommand{
	getCommand:     {getCommand, 1},
	putCommand:     {putCommand, 1},
	serveCommand:   {serveCommand, 1},
	ciphersCommand: {ciphersCommand, 1},
}

var CommandDescription = map[string]string{
	getCommand:     "perform an HTTP GET command to measure download speed",
	putCommand:     "perform an HTTP POST command to measure upload speed",
	serveCommand:   "launch a standard web server supporting GET and POST commands of arbitraty sizes",
	ciphersCommand: "list cipher suites supported by the target server(s)",
}

// global flags
var OptTLSSelf = false

// common GET/PUT flags that need treatment
type httpFlags struct {
	Timeout uint
	Delay   uint
	UseIPv4 bool
	UseIPv6 bool
}

// declare common GET/PUT flags
func httpCommonFlagSet(command string, errorHandling flag.ErrorHandling, job *HTTPjob) (*flag.FlagSet, *httpFlags) {
	fs := flag.NewFlagSet(command, errorHandling)
	fs.IntVar(&job.Repeat, "n", 1, "number of times to process an item")
	fs.StringVar(&job.Host, "a", "", "specific local address to use")
	fs.BoolVar(&job.SkipTLSCheck, "k", false, "skip certificates validation")
	fs.BoolVar(&job.UseTLSSelfConfig, "self", OptTLSSelf, "use self-signed TLS certificate")
	fs.BoolVar(&job.UseHTTP11, "http11", false, "force using HTTP 1.1")
	fs.BoolVar(&job.H2C, "h2c", false, "force using HTTP/2 Cleartext (H2C) mode")
	fs.StringVar(&job.UserAgent, "u", "nspeed.app/"+Version, "user-agent")

	hf := httpFlags{}
	fs.UintVar(&hf.Timeout, "t", 8, "timeout")
	fs.UintVar(&hf.Delay, "w", 0, "delay in seconds to wait before starting the command")
	fs.BoolVar(&hf.UseIPv4, "4", false, "use IPv4 only")
	fs.BoolVar(&hf.UseIPv6, "6", false, "use IPv6 only")

	return fs, &hf
}

// parse GET/PUT flags
func httpParse(fs *flag.FlagSet, hf *httpFlags, args []string, job *HTTPjob) error {

	if err := fs.Parse(args); err != nil {
		return err
	}

	// httpFlags (flags that need convertion or extra controls)
	job.Timeout = time.Duration(hf.Timeout) * time.Second
	job.Delay = time.Duration(hf.Delay) * time.Second

	if hf.UseIPv4 && hf.UseIPv6 {
		fs.Usage()
		return errors.New("specify IPv4 or IPv6 but not both")
	}
	if hf.UseIPv4 {
		job.IPversion = 4
	}
	if hf.UseIPv6 {
		job.IPversion = 6
	}

	return nil
}

func ParseCommand(command string, args []string) ([]string, Job, error) {
	switch command {
	case getCommand:
		j := HTTPjob{}
		j.Command = "GET"
		fs, hf := httpCommonFlagSet(command, flag.ContinueOnError, &j)
		fs.Usage = func() {
			fmt.Fprintf(fs.Output(), "Usage of %s: ", getCommand)
			fmt.Fprintf(fs.Output(), "  %s [options] url\n", getCommand)
			fmt.Fprintf(fs.Output(), "\nAvailable options:\n")
			fs.PrintDefaults()
			fmt.Fprintf(fs.Output(), "\n")
		}

		if err := httpParse(fs, hf, args, &j); err != nil {
			return args, nil, err
		}

		args = fs.Args()
		if len(args) == 0 {
			fs.Usage()
			return args, nil, errors.New("missing argument")
		}
		j.Target = args[0]

		args = args[1:]
		return args, &j, nil

	case putCommand:
		j := HTTPjob{}
		j.Command = "POST"
		fs, hf := httpCommonFlagSet(command, flag.ContinueOnError, &j)
		fs.Usage = func() {
			fmt.Fprintf(fs.Output(), "Usage of %s: ", putCommand)
			fmt.Fprintf(fs.Output(), "  %s [options] url size\n", putCommand)
			fmt.Fprintf(fs.Output(), "size is a number of bytes with an optionnal units like \"100k\" (100x1000 bytes) or \"100K\" (100x1024 bytes)\n")
			fmt.Fprintf(fs.Output(), "supported units: kmgtpe (x1000 units) and KMGTPE (x1024 units)\n")
			fmt.Fprintf(fs.Output(), "\nAvailable options:\n")
			fs.PrintDefaults()
			fmt.Fprintf(fs.Output(), "\n")
		}
		fs.StringVar(&j.ContentType, "ct", "application/octet-stream", "content type")

		if err := httpParse(fs, hf, args, &j); err != nil {
			return args, nil, err
		}

		args = fs.Args()
		if len(args) < 2 {
			fs.Usage()
			return args, nil, errors.New("missing arguments")
		}

		j.Target = args[0]

		cs, err := ParseByteUnits(args[1])
		if err != nil {
			return args, nil, err
		}
		j.ContentSize = int64(cs)
		args = args[2:]
		return args, &j, nil

	case serveCommand:
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		host := &NSpeedHost{}
		fs.Usage = func() {
			fmt.Fprintf(fs.Output(), "Usage of %s: ", serveCommand)
			fmt.Fprintf(fs.Output(), "  %s [options]\n", serveCommand)
			fmt.Fprintf(fs.Output(), "\nAvailable options:\n")
			fs.PrintDefaults()
			fmt.Fprintf(fs.Output(), "\n")
		}

		fs.StringVar(&host.Host, "a", "localhost", "Hostname or ip address to listen (use \"\" for all interfaces/all addresses)")
		//flag.Lookup("address").NoOptDefVal = ""
		useIPv4 := fs.Bool("4", false, "use IPv4 only")
		useIPv6 := fs.Bool("6", false, "use IPv6 only")
		fs.IntVar(&host.Port, "p", DefaultPort, "Port to listen on")
		maxsize := fs.String("s", FormatByteDecimalUnits(DefaultMaxBodySize), "Maximum size of response, in bytes or units of bytes")
		maxtime := fs.Uint("t", uint(DefaultMaxDuration.Seconds()), "Maximum duration a response may take")
		fs.DurationVar(&host.RunDuration, "d", 0, "duration after which the server shutdown (can use duration units: s,m,h)")
		fs.IntVar(&host.MaxRequests, "n", 0, "number of requests after which  the server shutdown (0 = infinite)")
		fs.StringVar(&host.TLSCertFile, "cert", "", "TLS certificate file")
		fs.StringVar(&host.TLSKeyFile, "key", "", "TLS Key file")
		fs.BoolVar(&host.UseTLSSelfConfig, "self", false, "listen in HTTPS using a self-signed TLS certificate")
		fs.StringVar(&host.Directory, "dir", "", "directory to serve content from (url will be /dir)")
		fs.BoolVar(&host.H2C, "h2c", false, "support also HTTP/2 Cleartext (H2C) mode")
		if err := fs.Parse(args); err != nil {
			return args, nil, err
		}

		//
		host.MaxDuration = time.Duration(*maxtime) * time.Second

		// force IPv4 or IPv6
		if *useIPv4 && *useIPv6 {
			fs.Usage()
			return args, nil, errors.New("specify IPv4 or IPv6 but not both")
		}
		if *useIPv4 {
			host.ForceIPVersion = 4
		}
		if *useIPv6 {
			host.ForceIPVersion = 6
		}

		ms, err := ParseByteUnits(*maxsize)
		if err != nil {
			fs.Usage()
			return args, nil, err
		}
		host.MaxBodySize = int64(ms)

		host.ServeTLS = host.UseTLSSelfConfig
		if host.TLSCertFile != "" || host.TLSKeyFile != "" {
			if _, err := os.Stat(host.TLSCertFile); os.IsNotExist(err) {
				return args, nil, errors.New("bad or missing TLS certificate file: " + host.TLSCertFile)
			}
			if _, err := os.Stat(host.TLSKeyFile); os.IsNotExist(err) {
				return args, nil, errors.New("bad or missing TLS key file: " + host.TLSKeyFile)
			}
			host.ServeTLS = true
		}

		args = fs.Args()
		j := NSpeedHTTPServer{Parameters: host}
		return args, &j, nil

	case ciphersCommand:
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		j := NSpeedCiphers{}

		fs.UintVar(&j.Delay, "w", 0, "delay in seconds to wait before starting the command")

		fs.Usage = func() {
			fmt.Fprintf(fs.Output(), "Usage of %s: ", ciphersCommand)
			fmt.Fprintf(fs.Output(), "  %s hostname[:port]\n", ciphersCommand)
			fmt.Fprintf(fs.Output(), "argument is a hostname or an ip address with an optionnal port\n")
			fmt.Fprintf(fs.Output(), "if no port is specified, 443 will be used\n")
			fmt.Fprintf(fs.Output(), "\nAvailable options:\n")
			fs.PrintDefaults()
			fmt.Fprintf(fs.Output(), "\n")
		}
		if err := fs.Parse(args); err != nil {
			return args, nil, err
		}
		args = fs.Args()
		if len(args) < 1 {
			fs.Usage()
			return args, nil, errors.New("missing target")
		}
		j.Target = args[0]
		args = args[1:]
		return args, &j, nil
	default:
		return args, nil, errors.New("unknown command")
	}
}
