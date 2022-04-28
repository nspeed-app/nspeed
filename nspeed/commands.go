package nspeed

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

// commands
type CommandId int

const getCommand = "get"
const putCommand = "put"
const postCommand = "post"
const serveCommand = "server"
const ciphersCommand = "ciphers"
const apiCommand = "api"

// const latencyCommand = "latency"
// const tracerouteCommand = "traceroute"

// supported commands, expand has needed
// Set is a set number. Commands in the same set can be mixed together.
// Commands in the "0" set can only be called alone and once
// FromAPI: true is command can be called using the API
type Command struct {
	Name    string
	Set     int
	FromAPI bool
}

// list of supported commands.
var Commands = map[string]Command{
	getCommand:     {getCommand, 1, true},
	putCommand:     {putCommand, 1, true},
	postCommand:    {postCommand, 1, true},
	serveCommand:   {serveCommand, 1, true},
	ciphersCommand: {ciphersCommand, 0, false},
	// latencyCommand:    {latencyCommand, 1, true},
	// tracerouteCommand: {tracerouteCommand, 2, true},
	apiCommand: {apiCommand, 1, false},
}

var CommandDescriptions = map[string]string{
	getCommand:     "perform an HTTP GET command to measure download speed",
	putCommand:     "perform an HTTP PUT command to measure upload speed",
	postCommand:    "perform an HTTP POST command to measure upload speed",
	serveCommand:   "launch a standard web server supporting GET and POST commands of arbitraty sizes",
	ciphersCommand: "list cipher suites supported by the target server(s)",
	// latencyCommand:    "measure latency to destination (not yet implemented)",
	// tracerouteCommand: "discover route to destination (not yet implemented)",
	apiCommand: "enable an API endpoint",
}

// global flags
var OptTLSSelf = false

// common GET/PUT flags that need treatment
type httpFlags struct {
	Timeout uint
	Delay   uint
	UseIPv4 bool
	UseIPv6 bool
	Url     string
}

// flags that need a name because they're used multiple time (with isFlagPassed notabily)
const urlFlag = "url"
const sizeFlag = "size"

// common GET/PUT flags
func httpCommonFlagSet(command string, errorHandling flag.ErrorHandling, job *JobHTTPClient) (*flag.FlagSet, *httpFlags) {
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
	fs.StringVar(&hf.Url, urlFlag, "", "target url for the command")
	return fs, &hf
}

// check is a flag was passed
func isFlagPassed(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// parse GET/PUT/POST flags
func httpParse(fs *flag.FlagSet, hf *httpFlags, args []string, job *JobHTTPClient) error {

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

// find command ,try full match first then partial unless ambiguous
func IsCommand(cmd string) (Command, error) {
	c, ok := Commands[cmd]
	if ok {
		return c, nil
	}

	found := ""
	for _, v := range Commands {
		if strings.HasPrefix(v.Name, cmd) {
			if found != "" {
				return Command{}, errors.New("ambiguous command name: " + cmd)
			} else {
				found = v.Name
			}
		}
	}
	if found != "" {
		return IsCommand(found)
	}
	return Command{}, errors.New("command not found: " + cmd)
}

// parse cmd line args for commands and their respective args
// todo: split this function into a func for each command and put it in Commands
func ParseCommand(command string, args []string, output io.Writer) ([]string, Job, error) {
	switch command {
	case getCommand:
		j := JobHTTPClient{}
		j.Command = "GET"
		fs, hf := httpCommonFlagSet(command, flag.ContinueOnError, &j)
		fs.SetOutput(output)
		fs.Usage = func() {
			fmt.Fprintf(fs.Output(), "Usage of %s: ", getCommand)
			fmt.Fprintf(fs.Output(), "  %s [options] [-url] url\n", getCommand)
			fmt.Fprintf(fs.Output(), "(keyword '-url' can be ommmited)\n")
			fmt.Fprintf(fs.Output(), "\nAvailable options:\n")
			fs.PrintDefaults()
			fmt.Fprintf(fs.Output(), "\n")
		}

		if err := httpParse(fs, hf, args, &j); err != nil {
			return args, nil, err
		}

		// "get target" or "get -url target"
		args = fs.Args()

		if isFlagPassed(fs, urlFlag) {
			j.Target = hf.Url
			if j.Target == "" {
				return args, nil, errors.New("empty url argument")
			}
		} else {
			if len(args) == 0 {
				fs.Usage()
				return args, nil, errors.New("missing url argument")
			}
			j.Target = args[0]
			args = args[1:]
		}

		return args, &j, nil

	case putCommand, postCommand:
		j := JobHTTPClient{}
		j.Command = "PUT"
		if command == postCommand {
			j.Command = "POST"
		}
		fs, hf := httpCommonFlagSet(command, flag.ContinueOnError, &j)
		fs.Usage = func() {
			fmt.Fprintf(fs.Output(), "Usage of %s: ", command)
			fmt.Fprintf(fs.Output(), "  %s [options] [-url] target [-size] size\n", command)
			fmt.Fprintf(fs.Output(), "size is a number of bytes with an optionnal units like \"100k\" (100x1000 bytes) or \"100K\" (100x1024 bytes)\n")
			fmt.Fprintf(fs.Output(), "supported units: kmgtpe (x1000 units) and KMGTPE (x1024 units)\n")
			fmt.Fprintf(fs.Output(), "keywords '-url' and '-size' can be ommmited\n")
			fmt.Fprintf(fs.Output(), "\nAvailable options:\n")
			fs.PrintDefaults()
			fmt.Fprintf(fs.Output(), "\n")
		}
		fs.StringVar(&j.ContentType, "ct", "application/octet-stream", "content type")
		var size string
		fs.StringVar(&size, sizeFlag, "", "size to upload")

		if err := httpParse(fs, hf, args, &j); err != nil {
			return args, nil, err
		}

		// put/post [-u] url [-s] size
		// since -u and -s are optionnal , we must check if they are here and if not advance args
		args = fs.Args()

		if isFlagPassed(fs, urlFlag) {
			j.Target = hf.Url
			if j.Target == "" {
				return args, nil, errors.New("empty url argument")
			}
		} else {
			if len(args) == 0 {
				fs.Usage()
				return args, nil, errors.New("missing url")
			}
			j.Target = args[0]
			args = args[1:]
		}

		if isFlagPassed(fs, sizeFlag) {
			if size == "" {
				return args, nil, errors.New("empty size argument")
			}
		} else {
			if len(args) == 0 {
				fs.Usage()
				return args, nil, errors.New("missing size argument")
			}
			size = args[0]
			args = args[1:]
		}

		cs, err := ParseByteUnits(size)
		if err != nil {
			return args, nil, err
		}
		j.ContentSize = int64(cs)

		u, err := url.Parse(j.Target)
		if err != nil {
			return args, nil, errors.New("bad url")
		}
		if command == putCommand {
			u.Path = path.Join(u.Path, FormatByteDecimalUnits(j.ContentSize))
			j.Target = u.String()
		}
		return args, &j, nil

	case serveCommand:
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		host := NewHostParameters()
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
		fs.BoolVar(&host.API, "api", false, "enable API endpoint (/api url)")
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
		j := JobHTTPServer{Parameters: host}
		return args, &j, nil

	case ciphersCommand:
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		j := JobCiphers{}

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
	case apiCommand:
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		host := NewHostParameters()
		j := APIHost{Parameters: host}

		fs.Usage = func() {
			fmt.Fprintf(fs.Output(), "Usage of %s: ", apiCommand)
			fmt.Fprintf(fs.Output(), "  %s [options]\n", apiCommand)
			fmt.Fprintf(fs.Output(), "\nAvailable options:\n")
			fs.PrintDefaults()
			fmt.Fprintf(fs.Output(), "\n")
		}

		fs.StringVar(&host.Host, "a", "localhost", "Hostname or ip address to listen (use \"\" for all interfaces/all addresses)")
		//flag.Lookup("address").NoOptDefVal = ""
		useIPv4 := fs.Bool("4", false, "use IPv4 only")
		useIPv6 := fs.Bool("6", false, "use IPv6 only")
		fs.IntVar(&host.Port, "p", DefaultPort, "Port to listen on")
		fs.StringVar(&host.TLSCertFile, "cert", "", "TLS certificate file")
		fs.StringVar(&host.TLSKeyFile, "key", "", "TLS Key file")
		fs.BoolVar(&host.UseTLSSelfConfig, "self", false, "listen in HTTPS using a self-signed TLS certificate")
		fs.BoolVar(&j.WebStats, "stats", false, "enable web stats at /rt url")
		fs.BoolVar(&j.WebStatsOnly, "statsonly", false, "enable web stats only (implies -stats)")
		fs.BoolVar(&j.WebStatsDebug, "statsdebug", false, "enable web stats debug (url is /debug/pprof/, GO specifics info, implies -stats)")

		if err := fs.Parse(args); err != nil {
			return args, nil, err
		}

		if j.WebStatsOnly || j.WebStatsDebug {
			j.WebStats = true
		}

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

		args = fs.Args()
		return args, &j, nil
	// case latencyCommand, tracerouteCommand:
	// 	return args, nil, errors.New("not yet implemented")
	default:
		return args, nil, errors.New("unknown command")
	}
}

// flag parsing - thx to https://github.com/artyom for hints
// make sure global args are parsed first
// this allow parsing of the form
// program [global flags] command1 .... commandN
// where commandX = command [local flags] [args]
func ParseCommands(rawArgs []string, output io.Writer, fromAPI bool) ([]Job, error) {
	var out []Job
	args := rawArgs
	cmdSet := -1 // no set yet

	for len(args) != 0 {
		command := args[0]
		args = args[1:]
		cmd, err := IsCommand(command)
		if err != nil {
			return nil, err
		}
		if cmd.Set == 0 && cmdSet != -1 {
			return nil, errors.New("only one command of this type is allowed")
		}
		if cmdSet == -1 {
			cmdSet = cmd.Set
		}
		if cmdSet != cmd.Set {
			return nil, fmt.Errorf("cannot mix this command (%s) with the previous one(s)", cmd.Name)
		}
		if fromAPI && !cmd.FromAPI {
			return nil, fmt.Errorf("this command (%s) can not be called from the API", cmd.Name)
		}
		reminder, job, err := ParseCommand(cmd.Name, args, output)
		if err != nil {
			return nil, err
		}
		args = reminder
		out = append(out, job)
	}
	return out, nil
}
