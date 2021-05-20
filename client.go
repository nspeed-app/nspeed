package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	_ "embed"

	"nspeed.app/nspeed"
)

// Version can be set at link time to override debug.BuildInfo.Main.Version,
// which is "(devel)" when building from within the module. See
// golang.org/issue/29814 and golang.org/issue/29228.
var Version string

//go:embed embed/localhost.pem
var TLSSelfSignedCert []byte

//go:embed embed/localhost-key.pem
var TLSSelfSignedKey []byte

//go:embed embed/localhost-CA.pem
var TLSSelfSignedCA []byte

// hacky hack
var ErrImmediateCommand = errors.New("immediate command executed")

// commands
const getCommand = "get"
const putCommand = "put"
const serveCommand = "server"
const cyphersCommand = "cyphers"

// supported commands, expand has needed
type NSpeedCommand struct {
	Name string
	Set  int
}

// list of supported commands. Value is the set number. Commands in the same set can be mixed together.
var implementedCommands = map[string]NSpeedCommand{
	getCommand:     {getCommand, 1},
	putCommand:     {putCommand, 1},
	serveCommand:   {serveCommand, 1},
	cyphersCommand: {cyphersCommand, 2},
}

var commandDescription = map[string]string{
	getCommand:     "perform an HTTP GET command to measure download speed",
	putCommand:     "perform an HTTP POST command to measure upload speed",
	serveCommand:   "launch a standard web server supporting GET and POST commands of arbitraty sizes",
	cyphersCommand: "list ciphersuites supported by the target server(s)",
}

// global options
var optCpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
var optMemprofile = flag.String("memprofile", "", "write memory profile to file")

var OptLogOutput = flag.String("log", "", "structured logging output filename (use - for stdout), implied -verbose")
var optVerbose = flag.Bool("verbose", false, "show progress & details")
var optDebug = flag.Bool("debug", false, "show more progress & details")
var optTick = flag.Int("tick", 1, "progress interval in seconds (only used in verbose mode)")
var optCPU = flag.Bool("cpu", false, "display cpu usage")

var optColor = flag.Bool("color", false, "use colors")

var versionFlag = flag.Bool("version", false, "display version")

var optTimeout = flag.Uint("timeout", 0, "default timeout in seconds, 0 for no timeout")

var optTest = flag.Bool("test", false, "just test arguments, do nothing")

var optTLSSelf = flag.Bool("self", false, "use embed self-signed certificate")

// flag parsing - thx to https://github.com/artyom for hints
// make sure global args are parsed first
// refactor: parsing should be done per command type
func parseJobs(rawArgs []string) ([]nspeed.Job, error) {
	var out []nspeed.Job
	args := rawArgs
	cmdSet := 0
	bImmediate := false

	for len(args) != 0 {
		command := args[0]
		args = args[1:]
		cmd, ok := implementedCommands[command]
		if !ok {
			return nil, errors.New("unknown command " + command)
		}
		if cmdSet == 0 {
			cmdSet = cmd.Set
		}
		if cmdSet != cmd.Set {
			return nil, errors.New("cannot mix these commands " + command)
		}
		switch command {
		case getCommand:
			fs := flag.NewFlagSet(command, flag.ContinueOnError)
			fs.Usage = func() {
				fmt.Fprintf(fs.Output(), "Usage of %s: ", getCommand)
				fmt.Fprintf(fs.Output(), "  %s [options] url\n", getCommand)
				fmt.Fprintf(fs.Output(), "\nAvailable options:\n")
				fs.PrintDefaults()
				fmt.Fprintf(fs.Output(), "\n")
			}

			repeat := fs.Uint("n", 1, "number of times to process an item")
			useIPv4 := fs.Bool("4", false, "use IPv4 only")
			useIPv6 := fs.Bool("6", false, "use IPv6 only")
			timeout := fs.Uint("t", 8, "timeout")
			host := fs.String("a", "", "specific local address to use")
			skipTLScert := fs.Bool("k", false, "skip certificates validation")
			useragent := fs.String("u", "nspeed.app/"+Version, "user-agent")
			waitdelay := fs.Uint("w", 0, "delay in seconds to wait before starting the command")
			useSelfCert := fs.Bool("self", *optTLSSelf, "use self-signed TLS certificate")
			useHTTP11 := fs.Bool("http11", false, "force using HTTP 1.1")

			if err := fs.Parse(args); err != nil {
				return nil, err
			}

			j := nspeed.HTTPjob{
				Command:   "GET",
				Timeout:   time.Duration(*timeout) * time.Second,
				Delay:     time.Duration(*waitdelay) * time.Second,
				UserAgent: *useragent,
				Repeat:    int(*repeat),
			}

			// force IPv4 or IPv6
			if *useIPv4 && *useIPv6 {
				fs.Usage()
				return nil, errors.New("specify IPv4 or IPv6 but not both")
			}
			if *useIPv4 {
				j.IPversion = 4
			}
			if *useIPv6 {
				j.IPversion = 6
			}
			j.SkipTLSCheck = *skipTLScert
			j.UseTLSSelfConfig = *useSelfCert
			j.UseHTTP11 = *useHTTP11

			log.Debug().Msgf("use self TLS: %t", j.UseTLSSelfConfig)

			args = fs.Args()
			if len(args) == 0 {
				fs.Usage()
				return nil, errors.New("missing argument")
			}
			j.Target = args[0]
			j.Host = *host

			args = args[1:]
			out = append(out, &j)
		case putCommand:
			fs := flag.NewFlagSet(command, flag.ContinueOnError)
			fs.Usage = func() {
				fmt.Fprintf(fs.Output(), "Usage of %s: ", putCommand)
				fmt.Fprintf(fs.Output(), "  %s [options] url size\n", putCommand)
				fmt.Fprintf(fs.Output(), "size is a number of bytes with an optionnal units like \"100k\" (100x1000 bytes) or \"100K\" (100x1024 bytes)\n")
				fmt.Fprintf(fs.Output(), "supported units: kmgtpe (x1000 units) and KMGTPE (x1024 units)\n")
				fmt.Fprintf(fs.Output(), "\nAvailable options:\n")
				fs.PrintDefaults()
				fmt.Fprintf(fs.Output(), "\n")
			}
			repeat := fs.Uint("n", 1, "number of times to process an item")
			useIPv4 := fs.Bool("4", false, "use IPv4 only")
			useIPv6 := fs.Bool("6", false, "use IPv6 only")
			timeout := fs.Uint("t", 8, "timeout")
			host := fs.String("a", "", "specific local address to use")
			skipTLScert := fs.Bool("k", false, "skip certificates validation")
			useragent := fs.String("u", "nspeed.app/"+Version, "user-agent")
			contentType := fs.String("ct", "application/octet-stream", "content type")
			waitdelay := fs.Uint("w", 0, "delay in seconds to wait before starting the command")
			useSelfCert := fs.Bool("self", *optTLSSelf, "use self-signed TLS certificate")
			useHTTP11 := fs.Bool("http11", false, "force using HTTP 1.1")
			if err := fs.Parse(args); err != nil {
				return nil, err
			}

			j := nspeed.HTTPjob{
				Command:     "POST",
				Timeout:     time.Duration(*timeout) * time.Second,
				Delay:       time.Duration(*waitdelay) * time.Second,
				UserAgent:   *useragent,
				ContentType: *contentType,
				Repeat:      int(*repeat),
			}
			// force IPv4 or IPv6
			if *useIPv4 && *useIPv6 {
				fs.Usage()
				return nil, errors.New("specify IPv4 or IPv6 but not both")
			}
			if *useIPv4 {
				j.IPversion = 4
			}
			if *useIPv6 {
				j.IPversion = 6
			}
			j.SkipTLSCheck = *skipTLScert
			j.UseTLSSelfConfig = *useSelfCert
			j.UseHTTP11 = *useHTTP11

			log.Debug().Msgf("use self TLS: %t", j.UseTLSSelfConfig)

			args = fs.Args()
			if len(args) < 2 {
				fs.Usage()
				return nil, errors.New("missing arguments")
			}

			j.Target = args[0]
			j.Host = *host

			cs, err := nspeed.ParseByteUnits(args[1])
			if err != nil {
				return nil, err
			}
			j.ContentSize = int64(cs)
			args = args[2:]
			out = append(out, &j)
		case serveCommand:
			fs := flag.NewFlagSet(command, flag.ContinueOnError)
			host := &nspeed.NSpeedHost{}
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
			fs.IntVar(&host.Port, "p", nspeed.DefaultPort, "Port to listen on")
			maxsize := fs.String("s", nspeed.FormatByteDecimalUnits(nspeed.DefaultMaxBodySize), "Maximum size of response, in bytes or units of bytes")
			maxtime := fs.Uint("t", uint(nspeed.DefaultMaxDuration.Seconds()), "Maximum duration a response may take")
			fs.DurationVar(&host.RunDuration, "d", 0, "duration after which the server shutdown (can use duration units: s,m,h)")
			fs.IntVar(&host.MaxRequests, "n", 0, "number of requests after which  the server shutdown (0 = infinite)")
			fs.StringVar(&host.TLSCertFile, "cert", "", "TLS certificate file")
			fs.StringVar(&host.TLSKeyFile, "key", "", "TLS Key file")
			fs.BoolVar(&host.UseTLSSelfConfig, "self", false, "listen in HTTPS using a self-signed TLS certificate")
			fs.StringVar(&host.Directory, "dir", "", "directory to serve content from (url will be /dir)")
			if err := fs.Parse(args); err != nil {
				return nil, err
			}

			//
			host.MaxDuration = time.Duration(*maxtime) * time.Second

			// force IPv4 or IPv6
			if *useIPv4 && *useIPv6 {
				fs.Usage()
				return nil, errors.New("specify IPv4 or IPv6 but not both")
			}
			if *useIPv4 {
				host.ForceIPVersion = 4
			}
			if *useIPv6 {
				host.ForceIPVersion = 6
			}

			ms, err := nspeed.ParseByteUnits(*maxsize)
			if err != nil {
				fs.Usage()
				return nil, err
			}
			host.MaxBodySize = int64(ms)

			host.ServeTLS = host.UseTLSSelfConfig
			if host.TLSCertFile != "" || host.TLSKeyFile != "" {
				if _, err := os.Stat(host.TLSCertFile); os.IsNotExist(err) {
					return nil, errors.New("bad or missing TLS certificate file: " + host.TLSCertFile)
				}
				if _, err := os.Stat(host.TLSKeyFile); os.IsNotExist(err) {
					return nil, errors.New("bad or missing TLS key file: " + host.TLSKeyFile)
				}
				host.ServeTLS = true
			}

			args = fs.Args()
			j := nspeed.NSpeedHTTPServer{Parameters: host}
			out = append(out, &j)
		case cyphersCommand:
			fs := flag.NewFlagSet(command, flag.ContinueOnError)
			fs.Usage = func() {
				fmt.Fprintf(fs.Output(), "Usage of %s: ", cyphersCommand)
				fmt.Fprintf(fs.Output(), "  %s hostname[:port]\n", cyphersCommand)
				fmt.Fprintf(fs.Output(), "argument is a hostname or an ip address with an optionnal port\n")
				fmt.Fprintf(fs.Output(), "if no port is specified, 443 will be used\n")
				// fmt.Fprintf(fs.Output(), "\nAvailable options:\n")
				fs.PrintDefaults()
				fmt.Fprintf(fs.Output(), "\n")
			}
			if err := fs.Parse(args); err != nil {
				return nil, err
			}
			args = fs.Args()
			if len(args) < 1 {
				fs.Usage()
				return nil, errors.New("missing target")
			}
			nspeed.PrintCyphers(args[0])
			bImmediate = true
			args = args[1:]
		default:
			return nil, errors.New("unknown command")
		}
	}
	if bImmediate {
		return out, ErrImmediateCommand
	} else {
		return out, nil
	}
}

// main
func main() {

	flag.CommandLine.Usage = func() {
		name := "nspeed" //todp
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n\n", name)
		fmt.Fprintf(flag.CommandLine.Output(), "  %s [global options] <command> ... <command>\n\n", name)
		fmt.Fprintf(flag.CommandLine.Output(), "Available commands:\n")
		for c := range implementedCommands {
			fmt.Fprintf(flag.CommandLine.Output(), "  - %-6s: %s\n", c, commandDescription[c])
		}
		fmt.Fprintf(flag.CommandLine.Output(), "\nuse: \"%s <command> -h\" to see each command specific arguments and options\n\n", name)
		fmt.Fprintf(flag.CommandLine.Output(), "Available global options:\n\n")
		flag.PrintDefaults()
	}

	flag.Parse() // parse global flags

	// todo: move this elsewhere ? use nspeed pkg version
	nspeed.ServerVersion = Version
	nspeed.ServerName = "NSpeed"

	if *versionFlag {
		if Version != "" {
			fmt.Println(Version)
			return
		}
		if buildInfo, ok := debug.ReadBuildInfo(); ok {
			fmt.Println(buildInfo.Main.Version)
			return
		}
		fmt.Println("(unknown)")
		return
	}

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
		fmt.Println("error: couldn't happen our self-signed CA")
		os.Exit(1)
	}

	nspeed.TLSSelfConfig = &tls.Config{
		Certificates: []tls.Certificate{certificate},
		RootCAs:      rootCAs}

	jobs, err := parseJobs(flag.Args())
	if err != nil && err != ErrImmediateCommand {
		fmt.Println(err)
		os.Exit(1)
	}

	// no command, return for now, later: check if launched from UI world (explorer, file manager, browser, etc)
	if len(jobs) == 0 && err != ErrImmediateCommand {
		flag.CommandLine.Usage()
		return
	}

	if *optCpuprofile != "" {
		f, err := os.Create(*optCpuprofile)
		if err != nil {
			log.Fatal().Err(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	// Logging stuff
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	consoleLevel := zerolog.WarnLevel

	if *optVerbose {
		consoleLevel = zerolog.InfoLevel
	}

	if *optDebug {
		consoleLevel = zerolog.DebugLevel
	}

	// console output, stderr, level is consoleLevel which depends on flags
	consoleWriter := nspeed.CreateFilteredWriter(zerolog.MultiLevelWriter(zerolog.ConsoleWriter{Out: os.Stderr, NoColor: !*optColor}), consoleLevel, false)

	// file output, "-" = stdout or filename
	// level is always info (for now)
	// todo : remove 'level'
	if *OptLogOutput != "" {
		f := os.Stdout
		if *OptLogOutput != "-" {
			var err error
			f, err = os.Create(*OptLogOutput)
			if err != nil {
				panic(err)
			}
		}
		fileWriter := nspeed.CreateFilteredWriter(zerolog.MultiLevelWriter(f), zerolog.InfoLevel, true)
		log.Logger = log.Output(zerolog.MultiLevelWriter(fileWriter, consoleWriter))
	} else {
		log.Logger = log.Output(consoleWriter)
	}

	log.Debug().Msgf("max cores = %d", nspeed.MaxParallelism())

	jobs = nspeed.ExpandJobs(jobs)

	njobs := len(jobs)
	results := make(chan *nspeed.WorkerMetrics, njobs)
	wcWorkers := make([]*nspeed.WorkerMetrics, njobs)

	//IsInfinite := false
	fmt.Println("Jobs:")
	for idx, job := range jobs {
		wm := nspeed.WorkerMetrics{
			Done:       false,
			Expired:    false,
			Index:      idx,
			Target:     job.GetTarget(),
			ResultChan: results,
			LogLevel:   consoleLevel, //todo
			Logger:     log.With().Int("job", idx).Logger(),
			Job:        job}
		wcWorkers[idx] = &wm
		fmt.Printf("  %d %#v\n", idx, job)
		// if job.IsInfinite() {
		// 	IsInfinite = true
		// }

		// todo check valid url, prefetch, do dns if needed, add context ? etc
	}

	// global timeout
	ctx := context.Background()
	if *optTimeout > 0 {
		newctx, cancel := context.WithTimeout(ctx, time.Duration(*optTimeout)*time.Second)
		defer cancel()
		ctx = newctx
	}

	// launch a go routine for each jobs
	var wg sync.WaitGroup
	wg.Add(njobs)
	for _, wm := range wcWorkers {
		log.Debug().Msgf("spawning workload %d %s %t", wm.Index, wm.Target, wm.Done)
		go func(w *nspeed.WorkerMetrics) {
			defer wg.Done()
			err := w.Job.Start(ctx, w)
			if err != nil {
				log.Error().Msgf("error %s for %s", err, w.Target)
				return
			}
		}(wm)
	}

	// main thread loop
	// - wait for (partial) result from workers
	// - wait for progress tick and print results
	// - wait for all workers to have ended or timeout'ed
	// - handle ctrl-c & errors

	cWaitgroupEnded := make(chan struct{})
	go func() {
		defer close(cWaitgroupEnded)
		wg.Wait()
		log.Debug().Msg("wait group ended")
	}()

	rc := njobs

	done := false
	if *optTest {
		done = true
	}

	progressTickerChannel := make(<-chan time.Time) // forever
	progressCPUChannel := make(<-chan time.Time)    // forever
	if *optCPU {
		progressCPUTicker := time.NewTicker(time.Duration(*optTick) * time.Second) // todo: pass value
		progressCPUChannel = progressCPUTicker.C
	}
	if zerolog.GlobalLevel() <= zerolog.InfoLevel {
		progressTicker := time.NewTicker(time.Duration(*optTick) * time.Second)
		progressTickerChannel = progressTicker.C
	}

	for !done {
		select {
		case <-progressCPUChannel:
			if *optCPU {
				cpuload := nspeed.GetCPULoad(*optColor)
				// we dont log.Info() this to avoid sending it to the file
				log.Log().Msgf("%s, active jobs: %d / active goroutines: %d", cpuload, rc, runtime.NumGoroutine())
			}

		case <-progressTickerChannel:
			// time critical: warning for overlap/backlogs if what's done here takes too long
			// this section assumes loglevel >= info
			for _, wm := range wcWorkers {
				if !wm.Done && (wm.LogLevel == zerolog.DebugLevel) {
					wm.LogProgress(&wm.Logger)
				}
			}
			PrintMemUsage(zerolog.GlobalLevel())
		case wm := <-results:
			rc--
			if wm.Done {
				log.Debug().Msgf("job %d ended - (on timeout: %t) - (%d remaining active goroutines)", wm.Index, wm.Expired, runtime.NumGoroutine())
			}
		case <-cWaitgroupEnded:
			log.Debug().Msgf("all workers done (%d remaining active goroutines)", runtime.NumGoroutine())
			done = true
		}
	}
	log.Debug().Msgf("main loop ended. active jobs: %d / active goroutines: %d", rc, runtime.NumGoroutine())
	log.Debug().Msg("sleeping one second")
	time.Sleep(1 * time.Second)

	// main speed report
	var elapsedTime time.Duration = 0
	var totalRead int64 = 0
	var totalWrite int64 = 0

	w := tabwriter.NewWriter(os.Stderr, 0, 2, 1, ' ', tabwriter.AlignRight|tabwriter.Debug)
	nspeed.InitSpeedReport(w)
	for _, wm := range wcWorkers {
		if wm.Valid {
			totalRead += wm.TotalRead
			totalWrite += wm.TotalWrite
			if wm.ElapsedTime > elapsedTime {
				elapsedTime = wm.ElapsedTime
			}
			nspeed.SpeedReport(w, fmt.Sprintf("Job %d", wm.Index), wm.ElapsedTime, wm.TotalRead, wm.TotalWrite, wm.Job.PrintString())
		} else {
			log.Warn().Err(wm.Error).Msgf("job %d", wm.Index)
		}
	}
	if totalRead == 0 && totalWrite == 0 {
		fmt.Println("nothing to report")
	} else {
		nspeed.SpeedReport(w, "Average", elapsedTime, totalRead, totalWrite, "")
		w.Flush()
	}
	// todo: defer this block
	if *optMemprofile != "" {
		f, err := os.Create(*optMemprofile)
		if err != nil {
			log.Fatal().Msgf("could not create memory profile: %v", err)
		}
		defer f.Close() // error handling omitted for example
		runtime.GC()    // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal().Msgf("could not write memory profile: %v", err)
		}
	}
}

// debug stuff

func PrintMemUsage(level zerolog.Level) {
	if level >= zerolog.InfoLevel {
		return
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// For info on each, see: https://golang.org/pkg/runtime/#MemStats
	log.Debug().Uint64("Alloc", m.Alloc).
		Uint64("TotalAlloc", m.TotalAlloc).
		Uint64("Sys", m.Sys).
		Uint64("Alloc(MiB)", bToMb(m.Alloc)).
		Uint64("TotalAlloc(MiB)", bToMb(m.TotalAlloc)).
		Uint64("Sys", bToMb(m.Sys)).
		Uint32("NumGC", m.NumGC).
		Send()
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}
