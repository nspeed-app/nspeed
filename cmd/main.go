package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"nspeed.app/nspeed"
	"nspeed.app/utils"
)

// global flags
var optCpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
var optMemprofile = flag.String("memprofile", "", "write memory profile to file")

var optLogOutput = flag.String("log", "", "structured logging output filename (use - for stdout), imply -verbose")
var optVerbose = flag.Bool("verbose", false, "show progress & details")
var optDebug = flag.Bool("debug", false, "show even more progress & details")
var optTrace = flag.Bool("trace", false, "maximum debug and tracing informations")

var optProgressRate = flag.Duration("tick", 1*time.Second, "display progress rate in duration (used for -debug, -trace and webstats")
var optSamplingRate = flag.Duration("rate", 1*time.Second, "data sampling rate in duration (used for -cpu and time series generation)")
var optCPU = flag.Bool("cpu", false, "display cpu usage (use -rate for sampling rate")

var optColor = flag.Bool("color", false, "use colors in text output")

var optVersion = flag.Bool("version", false, "display version (add \"all\" for more build and runtime informations)")
var optInfo = flag.Bool("info", false, "display informations about the host")

var optTimeout = flag.Uint("timeout", 0, "default global timeout in seconds, 0 for no timeout")

var optTest = flag.Bool("test", false, "just test arguments, do nothing")

var optTLSSelf = flag.Bool("self", false, "use embed self-signed certificate")

// var optHTMLReport = flag.String("html", "", "output stats in a html file (use - for stdout)")
var optJSONReport = flag.String("json", "", "output stats in a json file (use - for stdout)")
var optTextReport = flag.String("text", "-", "output stats as text (use - for stdout)")

var optPreDelay = flag.Duration("pre", 0, "duration to wait before starting commands")
var optPostDelay = flag.Duration("post", 0, "duration to wait after all commands ended")

// main
func Main() {

	flag.CommandLine.Usage = func() {
		name := "nspeed" //todo
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

	if *optInfo {
		fmt.Println(string(nspeed.MachineInfoJSON))
		return
	}

	if *optVersion {
		bi, ok := debug.ReadBuildInfo()
		if !ok {
			fmt.Println("no build info found!")
			os.Exit(1)
		}
		fmt.Printf("NSpeed version: %s\n", nspeed.Version)
		// this is a bit dirty since we don't use the flag package to parse '-version all'
		if flag.Arg(0) != "all" {
			return
		}

		fmt.Printf("Go version: %s\n", runtime.Version())
		fmt.Printf("OS Version: %s\n", runtime.GOOS)
		fmt.Printf("Arch Version: %s\n", runtime.GOARCH)
		fmt.Printf("BuildInfo:\n%s\n", bi.String())
		// for _, v := range bi.Settings {
		// 	if v.Key == "vcs.tag" {
		// 		fmt.Println("vcs.tag found: ", v.Value)
		// 		bi.Main.Version = v.Value
		// 		break
		// 	}
		// }
		return
	}

	//todo: up to here

	if *optCpuprofile != "" {
		f, err := os.Create(*optCpuprofile)
		if err != nil {
			log.Fatal().Err(err)
		}
		_ = pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	// Logging stuff
	consoleLevel := zerolog.WarnLevel

	if *optVerbose {
		consoleLevel = zerolog.InfoLevel
	}
	if *optDebug {
		consoleLevel = zerolog.DebugLevel
	}
	if *optTrace {
		consoleLevel = zerolog.TraceLevel
	}
	zerolog.SetGlobalLevel(consoleLevel)

	// remove these ugly "???" from zerolog level formatter

	formatLevel := func(i any) string {
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
	if *optLogOutput != "" {
		f, err := utils.FileCreate(*optLogOutput)
		if err != nil {
			panic("error creating log file!" + err.Error())
		}
		fileWriter := nspeed.CreateFilteredWriter(zerolog.MultiLevelWriter(f), zerolog.InfoLevel, true)
		log.Logger = log.Output(zerolog.MultiLevelWriter(fileWriter, consoleWriter))
	} else {
		log.Logger = log.Output(consoleWriter)
	}

	jl := nspeed.NewJoblist()
	err := nspeed.ParseCommands(flag.Args(), jl, nil, false)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// no command, return for now, later: check if launched from UI world (explorer, file manager, browser, etc)
	if len(jl.Jobs) == 0 {
		flag.CommandLine.Usage()
		return
	}

	//html report: by default, add delays before & after
	// if *optHTMLReport != "" {
	// 	// add 1 seconds before  and 1 seconds after unless pre/post flags are set
	// 	if *optPreDelay == 0 {
	// 		*optPreDelay = 1 * time.Second
	// 	}
	// 	if *optPostDelay == 0 {
	// 		*optPostDelay = 1 * time.Second
	// 	}
	// }

	log.Debug().Msgf("max cores = %d", nspeed.MaxParallelism())
	nspeed.GlobalOptions = nspeed.Options{
		ProgressInterval: *optProgressRate,
		Tick:             *optSamplingRate,
		Timeout:          time.Duration(*optTimeout) * time.Second,
		Test:             *optTest,
		TLSSelf:          false,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// shutdownCh triggers graceful shutdown on SIGINT or SIGTERM
	// todo: use signal.NotifyContext  ?
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	//////////////////
	// main thread loop
	// - wait for (partial) result from workers
	// - wait for progress tick and print results
	// - wait for all workers to have ended or timeout'ed
	// - handle ctrl-c & errors

	errs := make(chan error, 1)

	progressCPUChannel := make(<-chan time.Time) // forever
	if *optCPU {
		progressCPUTicker := time.NewTicker(*optSamplingRate) // todo: pass value
		progressCPUChannel = progressCPUTicker.C
	}

	// pre delay
	if *optPreDelay != 0 {
		log.Warn().Msgf("pre-delay started - sleeping %s", *optPreDelay)
		select {
		case <-ctx.Done():
		case <-time.After(*optPreDelay):
		}
		log.Warn().Msgf("pre-delay ended")
	}

	waitChan := make(chan error)
	var wcWorkers []*nspeed.Metrics = nil

	rc := 0
	// RunJobs async
	progress := func(wms []*nspeed.Metrics) {
		newrc := 0
		for _, wm := range wms {
			if !wm.Done {
				newrc += 1
				if consoleLevel == zerolog.DebugLevel {
					wm.LogProgress(&wm.Logger)
				}
			}
		}
		log.Debug().Msgf("progress called (%d)", len(wms))
		rc = newrc
	}
	// dont call progress if quiet
	if consoleLevel > zerolog.InfoLevel && !*optCPU {
		progress = nil
	}

	go func() {
		// defer wg.Done()
		var err error
		wcWorkers, err = nspeed.RunJobs(jl, nspeed.GlobalOptions, ctx, progress)
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
				cpuload, err := nspeed.GetCPULoad(0, *optColor)
				if err != nil {
					log.Log().Err(err).Msgf("failed to get cpu load")
				}
				// we dont log.Info() this to avoid sending it to the file
				log.Log().Msgf("%s, active jobs: %d / active goroutines: %d", cpuload, rc, runtime.NumGoroutine())
			}
		case err := <-errs:
			log.Warn().Err(err).Msgf("error with webview")
			cancel()
			break mainloop
		case <-waitChan:
			log.Info().Msgf("all jobs ended")
			break mainloop
		}
	}
	log.Debug().Msgf("main loop ended. active jobs: %d / active goroutines: %d", rc, runtime.NumGoroutine())

	// pre delay
	if *optPostDelay != 0 && ctx.Err() == nil {
		log.Warn().Msgf("post-delay started - sleeping %s", *optPostDelay)
		select {
		case <-time.After(*optPostDelay):
		case sig := <-shutdownCh:
			log.Warn().Msgf("global shutdown started by signal: %s", sig)
			cancel()
		}
		log.Warn().Msgf("post-delay ended")
	}

	if *optPostDelay != 0 && ctx.Err() != nil {
		log.Debug().Msgf("post-delay skipped because ctx.Err = %s", ctx.Err())
	}

	// text report:
	if *optTextReport != "" {
		f, err := utils.FileCreate(*optTextReport)
		if err != nil {
			log.Fatal().Msgf("could not open text report file (%s): %v", *optTextReport, err)
		}
		report := nspeed.GenerateTextReport(wcWorkers, *optVerbose)
		_, err = f.WriteString(report)
		if err == nil {
			_, err = f.WriteString(nspeed.SeriesReport(wcWorkers))
		}
		if err != nil {
			log.Error().Err(err).Msgf("error writing text report")
		}
		// add history after
		err = f.Close()
		if err != nil {
			log.Error().Err(err).Msgf("error closing text report")
		}
	}

	// json report:
	if *optJSONReport != "" {
		f, err := utils.FileCreate(*optJSONReport)
		if err != nil {
			log.Fatal().Msgf("could not open json report file (%s): %v", *optJSONReport, err)
		}
		report := nspeed.GenerateJSONReport(wcWorkers)
		_, _ = f.WriteString(report)
		f.Close()
	}

	// html report:
	// if *optHTMLReport != "" {
	// 	f, err := nspeed.FileCreate(*optHTMLReport)
	// 	if err != nil {
	// 		log.Fatal().Msgf("could not open html report file (%s): %v", *optHTMLReport, err)
	// 	}
	// 	nspeed.GenerateHTMLReport(f, wcWorkers)
	// }

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
