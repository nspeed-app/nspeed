package nspeed

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"nspeed.app/humanize"
)

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

// command line parsers
func CmdClientParseGet(command string, cmdargs []string, jl *JobList, output io.Writer) ([]string, error) {
	fs, hf := clientCommonFlagSet(command, flag.ContinueOnError, output)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage of %s: ", getCommand)
		fmt.Fprintf(fs.Output(), "  %s [options] [-url] url\n", getCommand)
		fmt.Fprintf(fs.Output(), "(keyword '-url' can be ommmited)\n")
		fmt.Fprintf(fs.Output(), "\nAvailable options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(fs.Output(), "\n")
	}
	var size string
	fs.StringVar(&size, sizeFlag, "", "max size to receive")

	j := JobClient{JobData: JobData{jobList: jl}}
	j.Command = "GET"

	if err := clientParse(fs, hf, cmdargs, &j); err != nil {
		return cmdargs, err
	}

	// "get target" or "get -url target"
	args := fs.Args()

	cs, err := humanize.ParseByteUnits(size)
	if err != nil {
		return args, err
	}
	j.ContentSize = int64(cs)

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

	return args, jl.AddJobs(expandJobs(hf.Instances, &j)...)
}

func expandJobs(instances int, j *JobClient) []Job {
	if instances <= 1 {
		return []Job{j}
	}
	jobs := make([]Job, instances)
	for i := 0; i < instances; i++ {
		jc := *j
		jc.Instance = i + 1
		jobs[i] = &jc
	}
	return jobs
}

func CmdClientParsePutPost(command string, cmdargs []string, jl *JobList, output io.Writer) ([]string, error) {
	j := JobClient{JobData: JobData{jobList: jl}}
	j.Command = "PUT"
	if command == postCommand {
		j.Command = "POST"
	}
	fs, hf := clientCommonFlagSet(command, flag.ContinueOnError, output)
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

	if err := clientParse(fs, hf, cmdargs, &j); err != nil {
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

	cs, err := humanize.ParseByteUnits(size)
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
	return args, jl.AddJobs(expandJobs(hf.Instances, &j)...)
}

// common GET/PUT/POST flags
type clientFlags struct {
	Protocol    string
	Host        string
	Instances   int
	Timeout     uint
	DialTimeout time.Duration
	Delay       time.Duration
	UseIPv4     bool
	UseIPv6     bool
	ID          string
	RemoteHost  string
	// TLS
	SkipTLSCheck     bool
	UseTLSSelfConfig bool
	// HTTP spectific
	UserAgent string
	URL       string
	UseHTTP11 bool
	UseHTTP2  bool
	UseHTTP3  bool
	H2C       bool
	Repeat    uint
}

// common GET/PUT flags
func clientCommonFlagSet(command string, errorHandling flag.ErrorHandling, output io.Writer) (*flag.FlagSet, *clientFlags) {
	fs := flag.NewFlagSet(command, errorHandling)
	fs.SetOutput(output)

	hf := clientFlags{}

	fs.StringVar(&hf.Protocol, "P", "http", "communication protocol")
	fs.IntVar(&hf.Instances, "n", 1, "number of times to process an item")
	fs.StringVar(&hf.Host, "a", "", "specific local address or port to use (ip or ip:port or :port. ip can also be an interface name)")
	fs.BoolVar(&hf.SkipTLSCheck, "k", false, "skip certificates validation")
	fs.BoolVar(&hf.UseTLSSelfConfig, "self", OptTLSSelf, "use self-signed TLS certificate")

	fs.UintVar(&hf.Timeout, "m", 0, "maximum duration in seconds this command is allowed to take (default timeout is used)")
	fs.DurationVar(&hf.DialTimeout, "connect-timeout", 2*time.Second, "maximum duration allowed for the initial connection")
	fs.DurationVar(&hf.Delay, "w", 0, "delay to wait before starting the command")
	fs.BoolVar(&hf.UseIPv4, "4", false, "use IPv4 only")
	fs.BoolVar(&hf.UseIPv6, "6", false, "use IPv6 only")

	fs.StringVar(&hf.UserAgent, "u", "nspeed.app/"+Version, "user-agent")
	fs.BoolVar(&hf.UseHTTP11, "http1.1", false, "force using HTTP 1.1")
	fs.BoolVar(&hf.UseHTTP2, "http2", false, "force using HTTP/2")
	fs.BoolVar(&hf.H2C, "h2c", false, "force using HTTP/2 Cleartext (H2C) mode (implies -http2)")
	fs.BoolVar(&hf.UseHTTP3, "http3", false, "use HTTP3")
	fs.StringVar(&hf.URL, urlFlag, "", "target url for the command")
	fs.StringVar(&hf.ID, "id", "", "name/id of this command")
	fs.UintVar(&hf.Repeat, "r", 1, "number of repeat loops")

	//fs.StringVar(&hf.RemoteHost, "remote", "", "use a remote nspeed instance (hostname:port)")
	return fs, &hf
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
func clientParse(fs *flag.FlagSet, hf *clientFlags, args []string, job *JobClient) error {

	if err := fs.Parse(args); err != nil {
		return err
	}

	// straigthforward flags - add more checks/controls later
	job.Protocol = strings.ToUpper(hf.Protocol)
	job.Host = hf.Host
	job.UseTLSSelfConfig = hf.UseTLSSelfConfig
	job.SkipTLSCheck = hf.SkipTLSCheck
	job.UserAgent = hf.UserAgent
	job.Delay = hf.Delay

	// httpFlags (flags that need convertion or extra controls)
	job.Timeout = time.Duration(hf.Timeout) * time.Second

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
	// Repeat
	job.Repeat = hf.Repeat
	if hf.Repeat == 0 {
		return errors.New("repeat cannot be zero")
	}
	// ID
	job.SetID(hf.ID)
	return nil
}
