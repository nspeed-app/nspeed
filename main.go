package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/context"

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

// main
func main() {

	nspeed.Version = Version

	flag.CommandLine.Usage = func() {
		name := "nspeed" //todp
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n\n", name)
		fmt.Fprintf(flag.CommandLine.Output(), "  %s [global options] <command> ... <command>\n\n", name)
		fmt.Fprintf(flag.CommandLine.Output(), "Available commands:\n")
		for c := range nspeed.Commands {
			fmt.Fprintf(flag.CommandLine.Output(), "  - %-6s: %s\n", c, nspeed.CommandDescriptions[c])
		}
		fmt.Fprintf(flag.CommandLine.Output(), "\nuse: \"%s <command> -h\" to see each command specific arguments and options\n\n", name)
		fmt.Fprintf(flag.CommandLine.Output(), "Available global options:\n\n")
		flag.PrintDefaults()
	}

	flag.Parse() // parse global flags
	nspeed.OptTLSSelf = *optTLSSelf

	// todo: move this elsewhere ? use nspeed pkg version
	nspeed.ServerVersion = Version
	nspeed.ServerName = "NSpeed"

	// set version: todo, move this in nspeed package
	if Version == "" {
		if buildInfo, ok := debug.ReadBuildInfo(); ok {
			Version = buildInfo.Main.Version
		} else {
			Version = "(unknown)"
		}
		nspeed.Version = Version
	}

	if *versionFlag {
		fmt.Println(Version)
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
		fmt.Println("error: couldn't append our embedded self-signed CA")
		os.Exit(1)
	}

	nspeed.TLSSelfConfig = &tls.Config{
		Certificates: []tls.Certificate{certificate},
		RootCAs:      rootCAs}

	jobs, err := nspeed.ParseCommands(flag.Args(), nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// no command, return for now, later: check if launched from UI world (explorer, file manager, browser, etc)
	if len(jobs) == 0 {
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

	// remove these ugly "???" from zerolog level formatter

	formatLevel := func(i interface{}) string {
		if i == nil {
			return ""
		}
		// wish we could call the default formatter but it's not exported
		// so we lose colors for level
		return strings.ToUpper(fmt.Sprintf("| %-6s|", i))
	}

	// console output, stderr, level is consoleLevel which depends on flags
	consoleWriter := nspeed.CreateFilteredWriter(
		zerolog.MultiLevelWriter(zerolog.ConsoleWriter{
			Out:         os.Stderr,
			NoColor:     !*optColor,
			FormatLevel: formatLevel,
		}),
		consoleLevel,
		false)

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
	nspeed.GlobalOptions = nspeed.Options{
		ProgressInterval: *optTick,
		Timeout:          time.Duration(*optTimeout) * time.Second,
		Test:             *optTest,
		TLSSelf:          false,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// shutdownCh triggers graceful shutdown on SIGINT or SIGTERM
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	//////////////////
	// main thread loop
	// - wait for (partial) result from workers
	// - wait for progress tick and print results
	// - wait for all workers to have ended or timeout'ed
	// - handle ctrl-c & errors

	progressCPUChannel := make(<-chan time.Time) // forever
	if *optCPU {
		progressCPUTicker := time.NewTicker(time.Duration(*optTick) * time.Second) // todo: pass value
		progressCPUChannel = progressCPUTicker.C
	}

	rc := 0
	// var wg sync.WaitGroup
	// wg.Add(1)
	waitChan := make(chan error)
	var wcWorkers []*nspeed.Metrics = nil
	// RunJobs async
	go func() {
		// defer wg.Done()
		var err error = nil
		wcWorkers, err = nspeed.RunJobs(jobs, nspeed.GlobalOptions, ctx, func(wms []*nspeed.Metrics) {
			wcWorkers = wms
			if consoleLevel == zerolog.DebugLevel {
				for _, wm := range wcWorkers {
					if !wm.Done {
						wm.LogProgress(&wm.Logger)
					}
				}
			}
			log.Debug().Msgf("progress called (%d)", rc)
		})
		if err != nil {
			log.Warn().Msgf("error running jobs: %v", err)
		}
		waitChan <- err
	}()

mainloop:
	for {
		select {
		case sig := <-shutdownCh:
			log.Warn().Msgf("global shutdown started by signal: %s", sig)
			cancel()
			break mainloop
		case <-progressCPUChannel:
			if *optCPU {
				cpuload := nspeed.GetCPULoad(*optColor)
				// we dont log.Info() this to avoid sending it to the file
				log.Log().Msgf("%s, active jobs: %d / active goroutines: %d", cpuload, rc, runtime.NumGoroutine())
			}
		case <-waitChan:
			log.Warn().Msgf("all jobs ended:")
			break mainloop
		}
	}
	log.Debug().Msgf("main loop ended. active jobs: %d / active goroutines: %d", rc, runtime.NumGoroutine())
	log.Debug().Msg("sleeping one second")
	time.Sleep(1 * time.Second)
	/////////////////

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
			log.Warn().Err(wm.Error).Msgf("job %d ended", wm.Index)
		}
	}
	if totalRead == 0 && totalWrite == 0 {
		fmt.Println("no metrics to report")
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

// debug stuff - todo : move this elsewhere

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
