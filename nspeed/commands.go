package nspeed

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"nspeed.app/utils"
)

// commands
type CommandID int

const getCommand = "get"
const putCommand = "put"
const postCommand = "post"
const serveCommand = "server"
const ciphersCommand = "ciphers"
const apiCommand = "api"
const benchmarkCommand = "bench"
const fromCommand = "from"
const thenCommand = "then"

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
	apiCommand:       {apiCommand, 1, false},
	benchmarkCommand: {benchmarkCommand, 1, true},
	fromCommand:      {fromCommand, 1, false},
	thenCommand:      {thenCommand, 1, false},
}

var CommandDescriptions = map[string]string{
	getCommand:     "perform an HTTP GET command to measure download speed",
	putCommand:     "perform an HTTP PUT command to measure upload speed",
	postCommand:    "perform an HTTP POST command to measure upload speed",
	serveCommand:   "launch a standard web server supporting GET and POST commands of arbitraty sizes",
	ciphersCommand: "list cipher suites supported by the target server(s)",
	// latencyCommand:    "measure latency to destination (not yet implemented)",
	// tracerouteCommand: "discover route to destination (not yet implemented)",
	apiCommand:       "enable an API endpoint",
	benchmarkCommand: "perform preselected commands",
	fromCommand:      "read command from file (use - for stdin)",
	thenCommand:      "executes all previous commands then continue",
}

// global flags
var OptTLSSelf = false

// common GET/PUT flags that need treatment/checks
type httpFlags struct {
	Timeout     uint
	DialTimeout time.Duration
	Delay       uint
	UseIPv4     bool
	UseIPv6     bool
	URL         string
	UseHTTP11   bool
	UseHTTP2    bool
	UseHTTP3    bool
	H2C         bool
	ID          string
	RemoteHost  string
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
	fs.StringVar(&job.UserAgent, "u", "nspeed.app/"+Version, "user-agent")

	hf := httpFlags{}
	fs.UintVar(&hf.Timeout, "m", 8, "maximum duration in seconds the command is allowed to take")
	fs.DurationVar(&hf.DialTimeout, "connect-timeout", 2*time.Second, "maximum duration allowed for the initial connection")
	fs.UintVar(&hf.Delay, "w", 0, "delay in seconds to wait before starting the command")
	fs.BoolVar(&hf.UseIPv4, "4", false, "use IPv4 only")
	fs.BoolVar(&hf.UseIPv6, "6", false, "use IPv6 only")
	fs.BoolVar(&hf.UseHTTP11, "http1.1", false, "force using HTTP 1.1")
	fs.BoolVar(&hf.UseHTTP2, "http2", false, "force using HTTP/2")
	fs.BoolVar(&hf.H2C, "h2c", false, "force using HTTP/2 Cleartext (H2C) mode (implies -http2)")
	fs.BoolVar(&hf.UseHTTP3, "http3", false, "use HTTP3")
	fs.StringVar(&hf.URL, urlFlag, "", "target url for the command")
	fs.StringVar(&hf.ID, "id", "", "name/id of this command")

	//fs.StringVar(&hf.RemoteHost, "remote", "", "use a remote nspeed instance (hostname:port)")
	return fs, &hf
}

// IsFlagPassed check is a flag was passed
func IsFlagPassed(fs *flag.FlagSet, name string) bool {
	passed := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			passed = true
		}
	})
	return passed
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
		//fs.Usage()
		return errors.New("specify IPv4 or IPv6 but not both")
	}
	if hf.UseIPv4 {
		job.IPversion = 4
	}
	if hf.UseIPv6 {
		job.IPversion = 6
	}

	if hf.UseHTTP11 && hf.UseHTTP2 ||
		hf.UseHTTP11 && hf.UseHTTP3 ||
		hf.UseHTTP2 && hf.UseHTTP3 {
		return errors.New("specify only one HTTP version")
	}
	// 0 = any version
	job.HTTPVersion = 0
	if hf.UseHTTP11 {
		job.HTTPVersion = 1
	}
	if hf.UseHTTP2 {
		job.HTTPVersion = 2
	}
	if hf.UseHTTP3 {
		job.HTTPVersion = 3
	}
	// h2c implied 2
	if hf.H2C {
		if hf.H2C && !(job.HTTPVersion == 2 || job.HTTPVersion == 0) {
			return errors.New("H2C only work with -http2")
		}
		job.H2C = true
		job.HTTPVersion = 2
	}
	job.SetID(hf.ID)
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
			}
			found = v.Name

		}
	}
	if found != "" {
		return IsCommand(found)
	}
	return Command{}, errors.New("command not found: " + cmd)
}

// parse cmd line args for commands and their respective args
// todo: split this function into a func for each command and put it in Commands
func ParseCommand(command string, cmdargs []string, jl *JobList, output io.Writer) ([]string, error) {
	switch command {
	case getCommand:
		j := JobHTTPClient{jobList: jl}
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

		if err := httpParse(fs, hf, cmdargs, &j); err != nil {
			return cmdargs, err
		}

		// "get target" or "get -url target"
		args := fs.Args()

		if IsFlagPassed(fs, urlFlag) {
			j.Target = hf.URL
			if j.Target == "" {
				return args, errors.New("empty url argument")
			}
		} else {
			if len(args) == 0 {
				fs.Usage()
				return args, errors.New("missing url argument")
			}
			j.Target = args[0]
			args = args[1:]
		}

		jobs := make([]Job, j.Repeat)
		// clone j j.Repeat times
		for i := 0; i < j.Repeat; i++ {
			jc := j
			jobs[i] = &jc
		}
		return args, jl.AddJobs(jobs...)

	case putCommand, postCommand:
		j := JobHTTPClient{jobList: jl}
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

		if err := httpParse(fs, hf, cmdargs, &j); err != nil {
			return cmdargs, err
		}

		// put/post [-u] url [-s] size
		// since -u and -s are optionnal , we must check if they are here and if not advance args
		args := fs.Args()

		if IsFlagPassed(fs, urlFlag) {
			j.Target = hf.URL
			if j.Target == "" {
				return args, errors.New("empty url argument")
			}
		} else {
			if len(args) == 0 {
				fs.Usage()
				return args, errors.New("missing url")
			}
			j.Target = args[0]
			args = args[1:]
		}

		if IsFlagPassed(fs, sizeFlag) {
			if size == "" {
				return args, errors.New("empty size argument")
			}
		} else {
			if len(args) == 0 {
				fs.Usage()
				return args, errors.New("missing size argument")
			}
			size = args[0]
			args = args[1:]
		}

		cs, err := utils.ParseByteUnits(size)
		if err != nil {
			return args, err
		}
		j.ContentSize = int64(cs)

		// check target is url
		_, err = url.Parse(j.Target)
		if err != nil {
			return args, errors.New("bad url")
		}

		// PUT command special treatment. the 'path' is the size argument
		// if command == putCommand {
		// 	u.Path = path.Join(u.Path, FormatByteDecimalUnits(j.ContentSize))
		// 	j.Target = u.String()
		// }
		jobs := make([]Job, j.Repeat)
		// clone j j.Repeat times
		for i := 0; i < j.Repeat; i++ {
			jc := j
			jobs[i] = &jc
		}
		return args, jl.AddJobs(jobs...)

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
		fs.IntVar(&host.Port, "p", 0, "Port number to listen on")
		maxsize := fs.String("s", utils.FormatByteDecimalUnits(DefaultMaxBodySize), "Maximum size of response, in bytes or units of bytes")
		maxtime := fs.Uint("m", uint(DefaultMaxDuration.Seconds()), "Maximum duration in seconds a response may take")
		fs.DurationVar(&host.RunDuration, "d", 0, "duration after which the server shutdown (can use duration units: s,m,h)")
		fs.IntVar(&host.MaxRequests, "n", 0, "number of requests after which  the server shutdown (0 = run forever)")
		fs.StringVar(&host.TLSCertFile, "cert", "", "TLS certificate file")
		fs.StringVar(&host.TLSKeyFile, "key", "", "TLS Key file")
		fs.BoolVar(&host.UseTLSSelfConfig, "self", false, "listen in HTTPS using a self-signed TLS certificate")
		fs.StringVar(&host.Directory, "dir", "", "directory to serve content from (url will be /dir)")
		fs.BoolVar(&host.H2C, "h2c", false, "listen in HTTP/2 Cleartext (H2C) mode")
		fs.BoolVar(&host.HTTP11, "http1.1", false, "listen only in HTTP/1.1 (this will disable HTTP/2) - implies -self is no cert/key ")
		fs.BoolVar(&host.HTTP2, "http2", false, "listen only in HTTP/2 (this will disable HTTP/1.1) - implies -self is no cert/key ")
		fs.BoolVar(&host.HTTP3, "http3", false, "also listen using HTTP/3 on QUIC (udp - same port) - EXPERIMENTAL")
		fs.BoolVar(&host.API, "api", false, "enable API endpoint (activate /api url)")
		fs.StringVar(&host.AltSvc, "alt", "", "add alt-svc endpoints (use id names, comma separated)")
		id := fs.String("id", "", "unique ID/name of this server")
		if err := fs.Parse(cmdargs); err != nil {
			return cmdargs, err
		}

		//
		host.MaxDuration = time.Duration(*maxtime) * time.Second

		// force IPv4 or IPv6
		if *useIPv4 && *useIPv6 {
			fs.Usage()
			return cmdargs, errors.New("specify IPv4 or IPv6 but not both")
		}
		if *useIPv4 {
			host.ForceIPVersion = 4
		}
		if *useIPv6 {
			host.ForceIPVersion = 6
		}

		ms, err := utils.ParseByteUnits(*maxsize)
		if err != nil {
			fs.Usage()
			return cmdargs, err
		}
		host.MaxBodySize = int64(ms)

		host.ServeTLS = host.UseTLSSelfConfig
		if host.TLSCertFile != "" || host.TLSKeyFile != "" {
			if _, err := os.Stat(host.TLSCertFile); os.IsNotExist(err) {
				return cmdargs, errors.New("bad or missing TLS certificate file: " + host.TLSCertFile)
			}
			if _, err := os.Stat(host.TLSKeyFile); os.IsNotExist(err) {
				return cmdargs, errors.New("bad or missing TLS key file: " + host.TLSKeyFile)
			}
			host.ServeTLS = true
		}

		// force -self if http2 or http3 and no tls files
		if (host.HTTP11 || host.HTTP2 || host.HTTP3) && !host.ServeTLS {
			host.UseTLSSelfConfig = true
			host.ServeTLS = true
		}
		if host.HTTP11 && host.HTTP2 {
			return cmdargs, errors.New("can't use both -http1.1 and -http2")
		}

		// if encryption is on (-self, -http1.1, -http2, -http3 or -key/-cert) then don't allow h2c
		if host.ServeTLS && host.H2C {
			return cmdargs, errors.New("h2c not compatible with self/http1.1/http2/http3" + host.TLSKeyFile)
		}

		args := fs.Args()
		j := JobHTTPServer{Parameters: host, JobList: jl}
		j.SetID(*id)
		err = jl.AddJobs(&j)
		return args, err

	case ciphersCommand:
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		j := JobCiphers{}

		fs.UintVar(&j.Delay, "w", 0, "delay in seconds to wait before starting the command")

		fs.Usage = func() {
			fmt.Fprintf(fs.Output(), "Usage of %s: ", ciphersCommand)
			fmt.Fprintf(fs.Output(), "  %s hostname[:port]\n", ciphersCommand)
			fmt.Fprintf(fs.Output(), "argument is a hostname or an ip address with an optionnal port\n")
			fmt.Fprintf(fs.Output(), "if no port number is specified, 443 will be used\n")
			fmt.Fprintf(fs.Output(), "\nAvailable options:\n")
			fs.PrintDefaults()
			fmt.Fprintf(fs.Output(), "\n")
		}
		if err := fs.Parse(cmdargs); err != nil {
			return cmdargs, err
		}
		args := fs.Args()
		if len(args) < 1 {
			fs.Usage()
			return args, errors.New("missing target")
		}
		j.Target = args[0]
		args = args[1:]
		err := jl.AddJobs(&j)
		return args, err
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
		fs.IntVar(&host.Port, "p", 0, "Port number to listen on")
		fs.StringVar(&host.TLSCertFile, "cert", "", "TLS certificate file")
		fs.StringVar(&host.TLSKeyFile, "key", "", "TLS Key file")
		fs.BoolVar(&host.UseTLSSelfConfig, "self", false, "listen in HTTPS using a self-signed TLS certificate")
		fs.BoolVar(&j.WebStats, "stats", false, "enable web stats at /rt url")
		fs.BoolVar(&j.WebStatsOnly, "statsonly", false, "enable web stats only (implies -stats)")
		fs.BoolVar(&j.WebStatsDebug, "statsdebug", false, "enable web stats debug (GO specifics info, implies -stats)")
		fs.BoolVar(&j.OpenBrowser, "browse", false, "open browser (implies -stats) - work for localhost only for now (wip)")
		interfaces := fs.String("statsinterfaces", "", "network interfaces to include (comma separated - default : all - implies -stats)")

		if err := fs.Parse(cmdargs); err != nil {
			return cmdargs, err
		}

		if j.WebStatsOnly || j.WebStatsDebug || j.OpenBrowser || *interfaces != "" {
			j.WebStats = true
		}

		if *interfaces != "" {
			j.WebStatsInterface = strings.Split(*interfaces, ",")
		}
		// force IPv4 or IPv6
		if *useIPv4 && *useIPv6 {
			fs.Usage()
			return cmdargs, errors.New("specify IPv4 or IPv6 but not both")
		}
		if *useIPv4 {
			host.ForceIPVersion = 4
		}
		if *useIPv6 {
			host.ForceIPVersion = 6
		}

		args := fs.Args()
		err := jl.AddJobs(&j)
		return args, err

	case benchmarkCommand:
		fs := flag.NewFlagSet(command, flag.ContinueOnError)

		fs.Usage = func() {
			fmt.Fprintf(fs.Output(), "Usage of %s: ", benchmarkCommand)
			fmt.Fprintf(fs.Output(), "  %s [options] benchmark[,benchmark]\n", benchmarkCommand)
			fmt.Fprintf(fs.Output(), "\nAvailable benchmark:\n")

			for _, k := range benchmarKeys {
				fmt.Fprintf(fs.Output(), "  %-4s: %s\n", k, benchmarksTemplates[k].Name)
			}

			fmt.Fprintf(fs.Output(), "\nAvailable options:\n")
			fs.PrintDefaults()
			fmt.Fprintf(fs.Output(), "\n")
		}
		useIPv4 := fs.Bool("4", false, "use IPv4 only")
		useIPv6 := fs.Bool("6", false, "use IPv6 only")
		nStreams := fs.Int("n", 1, "number of streams")
		size := fs.String("s", "20g", "number of bytes")
		timeOut := fs.Uint("m", 8, "maximum duration in seconds the command is allowed to take")

		if err := fs.Parse(cmdargs); err != nil {
			return cmdargs, err
		}
		// force IPv4 or IPv6
		if *useIPv4 && *useIPv6 {
			fs.Usage()
			return cmdargs, errors.New("specify IPv4 or IPv6 but not both")
		}
		argsLine := fmt.Sprintf("-m %d", *timeOut)
		if *useIPv6 {
			argsLine = " -6"
		}
		if *useIPv4 {
			argsLine = " -4"
		}

		// we parse the size just to check it's valid
		_, err := utils.ParseByteUnits(*size)
		if err != nil {
			return cmdargs, err
		}

		args := fs.Args()
		if len(args) < 1 {
			fs.Usage()
			return args, errors.New("missing benchmark parameters")
		}
		benchs := strings.Split(args[0], ",")
		args = args[1:]

		id := 1
		values := BenchmarkArgs{
			Args: argsLine,
			N:    *nStreams,
			Size: *size,
			ID:   id,
		}
		for _, arg := range benchs {
			ct, ok := benchmarksTemplates[arg]
			if !ok {
				log.Fatal().Msgf("unknown benchmark: '%s'", arg)
			}
			tpl := template.New("T")
			t := template.Must(tpl.Parse(ct.Tpl))

			var c bytes.Buffer
			if err := t.Execute(&c, values); err != nil {
				log.Fatal().Err(err).Send()
			}
			cmd := strings.Fields(c.String())
			id++
			values.ID = id
			log.Info().Msgf("benchmark: %s", cmd)
			err := ParseCommands(cmd, jl, output, false)
			if err != nil {
				return args, err
			}
		}
		return args, nil
	case fromCommand:
		fs := flag.NewFlagSet(command, flag.ContinueOnError)

		fs.Usage = func() {
			fmt.Fprintf(fs.Output(), "Usage of %s: ", fromCommand)
			fmt.Fprintf(fs.Output(), "  %s filename\n", fromCommand)
			fs.PrintDefaults()
			fmt.Fprintf(fs.Output(), "\n")
		}
		if err := fs.Parse(cmdargs); err != nil {
			return cmdargs, err
		}
		args := fs.Args()
		if len(args) < 1 {
			fs.Usage()
			return args, errors.New("missing filename")
		}
		uri := args[0]
		args = args[1:]
		log.Debug().Msgf("fromCommand, url/file to process = %s", uri)

		// process file
		cmdFile := io.ReadCloser(os.Stdin)
		if uri != "-" {
			var err error
			cmdFile, err = utils.OpenURI(uri)
			if err != nil {
				return args, fmt.Errorf("can't open uri %s (%s)", uri, err)
			}
		}
		{
			defer cmdFile.Close()

			cmdScan := bufio.NewScanner(cmdFile)

			log.Warn().Msgf("reading commands from %s", uri)
			lineNumber := 1
			for cmdScan.Scan() {
				line, err := utils.ToArgv(cmdScan.Text())
				if err != nil {
					return args, err
				}
				err = ParseCommands(line, jl, output, false)
				if err != nil {
					return args, err
				}
				err = jl.AddBatch(&Batch{Name: fmt.Sprintf("line %d of %s", lineNumber, uri)})
				if err != nil {
					return args, err
				}
				lineNumber++
			}
		}
		return args, nil
	case thenCommand:
		fs := flag.NewFlagSet(command, flag.ContinueOnError)

		fs.Usage = func() {
			fmt.Fprintf(fs.Output(), "Usage of %s: ", thenCommand)
			fmt.Fprintf(fs.Output(), "  %s [options]\n", thenCommand)
			fmt.Fprintf(fs.Output(), "\nAvailable options:\n")
			fs.PrintDefaults()
			fmt.Fprintf(fs.Output(), "\n")
		}
		batch := Batch{}
		fs.DurationVar(&batch.PreDelay, "pre", 0, "duration to wait before starting commands")
		fs.DurationVar(&batch.PostDelay, "post", 0, "duration to wait after commands ended")
		fs.StringVar(&batch.Name, "n", "", "name of this batch")
		if err := fs.Parse(cmdargs); err != nil {
			return cmdargs, err
		}
		err := jl.AddBatch(&batch)
		if err != nil {
			return cmdargs, err
		}
		args := fs.Args()
		return args, nil
	// case latencyCommand, tracerouteCommand:
	// 	return args, nil, errors.New("not yet implemented")
	default:
		return cmdargs, errors.New("unknown command")
	}
}

// flag parsing - thx to https://github.com/artyom for hints
// make sure global args are parsed first
// this allow parsing of the form
// program [global flags] command1 .... commandN
// where commandX = command [local flags] [args]
func ParseCommands(rawArgs []string, joblist *JobList, output io.Writer, fromAPI bool) error {
	args := rawArgs
	cmdSet := -1 // no set yet

	for len(args) != 0 {
		command := args[0]
		if len(command) > 0 && command[0] == '#' {
			return nil
		}
		args = args[1:]
		cmd, err := IsCommand(command)
		if err != nil {
			return err
		}
		if cmd.Set == 0 && cmdSet != -1 {
			return errors.New("only one command of this type is allowed")
		}
		if cmdSet == -1 {
			cmdSet = cmd.Set
		}
		if cmdSet != cmd.Set {
			return fmt.Errorf("cannot mix this command (%s) with the previous one(s)", cmd.Name)
		}
		if fromAPI && !cmd.FromAPI {
			return fmt.Errorf("this command (%s) can not be called from the API", cmd.Name)
		}
		reminder, err := ParseCommand(cmd.Name, args, joblist, output)
		if err != nil {
			return err
		}
		args = reminder
	}
	return nil
}
