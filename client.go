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
	"strings"
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
func parseJobs(rawArgs []string) ([]nspeed.Job, error) {
	var out []nspeed.Job
	args := rawArgs
	cmdSet := 0

	for len(args) != 0 {
		command := args[0]
		args = args[1:]
		cmd, err := nspeed.IsCommand(command) //todo:  refine with unambiquitous  substring 'g' instead of get'
		if err != nil {
			return nil, err
		}
		if cmdSet == 0 {
			cmdSet = cmd.Set
		}
		if cmdSet != cmd.Set {
			return nil, errors.New("cannot mix this command (" + cmd.Name + ") with the previous one(s)")
		}
		reminder, job, err := nspeed.ParseCommand(cmd.Name, args)
		if err != nil {
			return nil, err
		}
		args = reminder
		out = append(out, job)
	}
	return out, nil
}

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
		// todo check valid url, prefetch, do dns if needed, add context ? etc
	}

	// global timeout
	ctx := context.Background()
	if *optTimeout > 0 {
		newctx, cancel := context.WithTimeout(ctx, time.Duration(*optTimeout)*time.Second)
		defer cancel()
		ctx = newctx
	}

	// create & init workers for jobs
	for _, wm := range wcWorkers {
		log.Debug().Msgf("creating workload %d %s %t", wm.Index, wm.Target, wm.Done)
		err := wm.Job.Init(wm)
		if err != nil {
			log.Error().Msgf("error %s for %s", err, wm.Target)
			return
		}
	}

	// run by launching a go routine for each job
	var wg sync.WaitGroup
	wg.Add(njobs)
	for _, wm := range wcWorkers {
		log.Debug().Msgf("spawning workload %d %s %t", wm.Index, wm.Target, wm.Done)
		go func(w *nspeed.WorkerMetrics) {
			defer wg.Done()
			err := w.Job.Run(ctx)
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
			log.Warn().Err(wm.Error).Msgf("job %d ended", wm.Index)
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
