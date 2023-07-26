package nspeed

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/rs/zerolog"
	"nspeed.app/utils"
)

// BigBuffer is used with io.CopyBuffer because of io.Copy internal buffer is too small.
// todo: we set a 4 MiB size. on small systems/embedded systems this could be an issue
// This buffer can be read/written by multiple goroutines at the same time so do not use the data in it.
// This will generate a data race
// io.CopyBuffer default size is 32 * 1024
var bigBuffer [1024 * 1024]byte
var BigBuffer = bigBuffer[:]

// BigChunk is a big array of random data
const MaxChunkSize = 1 * 1024 * 1024 // warning : 1 MiB //this will be allocated in memory
var BigChunk [MaxChunkSize]byte

// init a buffer of random values - callable only once
// thread safe
func InitBigChunk(seed int64) {
	rng := rand.New(rand.NewSource(seed))
	for i := int64(0); i < MaxChunkSize; i++ {
		BigChunk[i] = byte(rng.Intn(256))
	}
}

// called once
func init() {
	InitBigChunk(time.Now().Unix())
}

// mostly from go-httbin - todo: refactor / rewrite this

func logger(logger *zerolog.Logger, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//todo: this breaks websocket
		t := time.Now()
		h.ServeHTTP(w, r)
		logger.Info().
			Str("method", r.Method).
			Stringer("URL", r.URL).
			Dur("duration", time.Since(t))
	})
}

// limit inbound request size
func limitRequestSize(maxSize int64, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxSize)
		}
		h.ServeHTTP(w, r)
	})
}

func limitNumRequests(OnRequestStart func() bool, OnRequestEnd func(), h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if OnRequestStart() {
			h.ServeHTTP(w, r)
		} else {
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
		}
		OnRequestEnd()
	})
}

// todo: check if this is still needed,
// todo: add a CORS setting
func preflight(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		respHeader := w.Header()
		respHeader.Set("Access-Control-Allow-Origin", origin)
		respHeader.Set("Access-Control-Allow-Credentials", "true")
		respHeader.Set("Server", ServerName+" "+ServerVersion) //todo:move this elsewhere ?

		if r.Method == "OPTIONS" {
			w.Header().Set("Access-Control-Allow-Methods", "HEAD, GET, POST, PUT, OPTIONS") // , HEAD, PUT, DELETE, PATCH, ") // todo
			w.Header().Set("Access-Control-Max-Age", "3600")                                //todo
			if r.Header.Get("Access-Control-Request-Headers") != "" {
				w.Header().Set("Access-Control-Allow-Headers", r.Header.Get("Access-Control-Request-Headers"))
			}
			w.WriteHeader(200)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// command line parsers
func HTTPClientParseGet(command string, cmdargs []string, jl *JobList, output io.Writer) ([]string, error) {
	j := JobHTTPClient{JobData: JobData{jobList: jl}}
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
}
func HTTPClientParsePutPost(command string, cmdargs []string, jl *JobList, output io.Writer) ([]string, error) {
	j := JobHTTPClient{JobData: JobData{jobList: jl}}
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
}
func HTTPServerParse(command string, cmdargs []string, jl *JobList, output io.Writer) ([]string, error) {
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
}

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
		job.IPVersion = 4
	}
	if hf.UseIPv6 {
		job.IPVersion = 6
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
