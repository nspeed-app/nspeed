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

	"nspeed.app/chartjs"
	"nspeed.app/metric"
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
var optSamplingRate = flag.Duration("rate", 0, "data sampling rate in duration (used for time series generation)")
var optCPURate = flag.Duration("cpurate", 0, "set cpu rate (implies -cpu)")
var optCPU = flag.Bool("cpu", false, "display cpu usage (default is every second, use -cpurate to change)")

var optColor = flag.Bool("color", false, "use colors in text output")

var optVersion = flag.Bool("version", false, "display version (add \"all\" for more build and runtime informations)")
var optInfo = flag.Bool("info", false, "display informations about the host")

var optTimeout = flag.Uint("timeout", 0, "default global timeout in seconds, 0 for no timeout")

var optTest = flag.Bool("test", false, "just test arguments, do nothing")

var optTLSSelf = flag.Bool("self", false, "use embed self-signed certificate")

var optHTMLReport = flag.String("html", "", "output stats in a html file (use - for stdout)")
var optJSONReport = flag.String("json", "", "output stats in a json file (use - for stdout)")
var optTextReport = flag.String("text", "-", "output stats as text (use - for stdout)")
var optUngroup = flag.Bool("ungroup", false, "show members of groups")

var optPreDelay = flag.Duration("pre", 0, "duration to wait before starting commands")
var optPostDelay = flag.Duration("post", 0, "duration to wait after all commands ended")
var optName = flag.String("n", "", "name of the first batch")

var optLogTimestamps = flag.Bool("display-log-time", false, "display logging timestamps")
var optLogLevels = flag.Bool("display-log-level", false, "display logging levels (always true if -verbose/-debug/-trace)")

var optDNSServer = flag.String("dns-server", "", "specify a dns server to use (literal ip or literal ip:port)")

var optIPv4 = flag.Bool("4", false, "use IPv4 only")
var optIPv6 = flag.Bool("6", false, "use IPv6 only")

// todo: automate this.
// relative path of this file to go.mod
var thisFileRelToModule = "../.."

// Main as main
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

	//flag.Var(&optHostnames, "resolve", "Provide a custom address for a specific host and port pair (literal ip or literal ip:port)")

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
	var logHook zerolog.Hook
	consoleLevel := zerolog.WarnLevel

	var displayLogLevels = *optLogLevels

	if *optVerbose {
		displayLogLevels = true
		consoleLevel = zerolog.InfoLevel
	}
	if *optDebug {
		displayLogLevels = true
		consoleLevel = zerolog.DebugLevel
		logHook = nspeed.NewLineInfoHook(thisFileRelToModule)
	}
	if *optTrace {
		displayLogLevels = true
		consoleLevel = zerolog.TraceLevel
		logHook = nspeed.NewLineInfoHook(thisFileRelToModule)
	}
	zerolog.SetGlobalLevel(consoleLevel)

	// remove these ugly "???" from zerolog level formatter

	formatLevel := func(i any) string {
		if !displayLogLevels {
			return ""
		}
		if i == nil {
			return ""
		}
		// wish we could call the default formatter but it's not exported
		// so we lose colors for level
		return strings.ToUpper(fmt.Sprintf("| %-6s|", i))
	}
	// for no timestamps return ""
	formatTimestamp := func(i any) string {
		return ""
	}
	formatFieldValue := func(i any) string {
		if i != nil {
			return fmt.Sprintf("%s |", i)
		}
		return ""
	}
	if *optLogTimestamps {
		formatTimestamp = nil
	}
	// console output, stderr, level is consoleLevel which depends on flags

	consolePartsOrder := []string{
		zerolog.TimestampFieldName,
		zerolog.LevelFieldName,
		zerolog.CallerFieldName,
		"job",
		zerolog.MessageFieldName,
	}
	consoleWriter := nspeed.CreateFilteredWriter(
		zerolog.MultiLevelWriter(zerolog.ConsoleWriter{
			Out:              os.Stderr,
			NoColor:          !*optColor,
			FormatLevel:      formatLevel,
			TimeFormat:       time.TimeOnly,
			FormatTimestamp:  formatTimestamp,
			FormatFieldValue: formatFieldValue,
			PartsOrder:       consolePartsOrder,
			FieldsExclude:    []string{"job"},
		}),
		consoleLevel,
		false)

	// file output, "-" = stdout or filename
	// level is always info (for now)
	// todo : remove 'level'
	// todo: stderr
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
	if logHook != nil {
		log.Logger = log.Logger.Hook(logHook)
	}
	// force IPv4 or IPv6
	if *optIPv4 && *optIPv6 {
		log.Fatal().Msg("specify IPv4 or IPv6 but not both")
	}
	//
	if *optDNSServer != "" {
		if err := utils.SetDNSServer(*optDNSServer); err != nil {
			log.Fatal().Err(err).Msg("error setting dns server")
		}
	}

	// cpu sampling rate
	cpuRate := *optCPURate
	if *optCPU {
		if cpuRate == 0 {
			cpuRate = time.Second
		}
	}
	optIPVersion := 0
	if *optIPv4 {
		optIPVersion = 4
	}
	if *optIPv6 {
		optIPVersion = 6
	}
	nspeed.GlobalOptions = nspeed.Options{
		ProgressInterval: *optProgressRate,
		Tick:             *optSamplingRate,
		CPU:              cpuRate,
		Timeout:          time.Duration(*optTimeout) * time.Second,
		Test:             *optTest,
		TLSSelf:          false,
		IPVersion:        optIPVersion,
	}

	jl := nspeed.NewJoblist()
	err := jl.AddBatch(&nspeed.Batch{
		PreDelay:  *optPreDelay,
		PostDelay: *optPostDelay,
		Name:      *optName,
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	// todo: nspeed.GlobalOptions is used during ParseCommands (for ipversion mainly)
	err = nspeed.ParseCommands(flag.Args(), jl, nil, false)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// no command, return for now, later: check if launched from UI world (explorer, file manager, browser, etc)
	if len(jl.Jobs) == 0 {
		flag.CommandLine.Usage()
		return
	}

	log.Debug().Msgf("max cores = %d", nspeed.MaxParallelism())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// osInterrupt triggers graceful shutdown on SIGINT or SIGTERM
	// since we won't call signal.Reset(...), make sure all execution paths exit at some point
	// otherwise add signal.Reset(...) after post-delay handling code
	osInterrupt := make(chan os.Signal, 1)
	signal.Notify(osInterrupt, syscall.SIGINT, syscall.SIGTERM)

	errs := make(chan error, 1)

	// pre delay
	if *optPreDelay != 0 {
		log.Warn().Msgf("pre-delay started - sleeping %s", *optPreDelay)
		select {
		case <-ctx.Done():
		case <-time.After(*optPreDelay):
		case sig := <-osInterrupt:
			log.Warn().Msgf("global shutdown started by signal: %s", sig)
			return
		}
		log.Warn().Msgf("pre-delay ended")
	}

	printHeaders := true
	progressHandler := func(p any) {
		switch m := p.(type) {
		// this is bad, review/refactor
		case []*metric.Metric:
			rc := 0
			for _, wm := range m {
				if !wm.Done {
					rc++
					if consoleLevel == zerolog.TraceLevel {
						wm.LogProgress(wm.Logger())
					}
				}
			}
			log.Debug().Msgf("progress called %d jobs (%d)", len(m), rc)
		case nspeed.CPUMetric:
			if cpuRate > 0 { // if this doesn't match, we did something wrong
				cpuload, headers, _ := nspeed.PrintCPULoad(m.Pcts, *optColor)
				if printHeaders {
					log.Log().Msgf("%s jobs | threads|", headers)
					printHeaders = false
				}
				log.Log().Msgf("%s %-4d | %-4d   |", cpuload, m.ActiveJobs, runtime.NumGoroutine())
			}
		}
	}

	// go routine to call nspeed.RunJobs. We interop & sync with progress and waitChan
	// results will be in wcWorkers
	waitChan := make(chan error)
	progress := make(chan any)
	var wcWorkers metric.Metrics //todo: rename this var
	go func() {
		var err error
		wcWorkers, err = nspeed.RunJobs(ctx, jl, nspeed.GlobalOptions, progress)
		if err != nil {
			log.Warn().Msgf("error running jobs: %v", err)
		}
		waitChan <- err
	}()
	// assumes all jobs are running

	//////////////////
	// main thread loop
	// - wait for (partial) result from workers
	// - wait for progress tick and print results
	// - wait for all workers to have ended or timeout'ed
	// - handle ctrl-c & errors

mainloop:
	for {
		select {
		case m := <-progress:
			progressHandler(m)
		case sig := <-osInterrupt:
			log.Warn().Msgf("global shutdown started by signal: %s", sig)
			cancel() // this should end RunJobs which will signal waitChan
		case err := <-errs:
			log.Warn().Err(err).Msgf("error with webview")
			cancel()
			break mainloop
		case <-waitChan:
			log.Info().Msgf("all jobs ended")
			break mainloop
		}
	}
	log.Debug().Msgf("main loop ended. total results: %d / active goroutines: %d", len(wcWorkers), runtime.NumGoroutine())

	// post delay -todo share more code with pre delay
	if *optPostDelay != 0 && ctx.Err() == nil {
		log.Warn().Msgf("post-delay started - sleeping %s", *optPostDelay)
		select {
		case <-time.After(*optPostDelay):
		case sig := <-osInterrupt:
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
		report := metric.GenerateTextReport(jl.ResultBatches(&wcWorkers), &wcWorkers, *optUngroup)
		_, err = f.WriteString(report)
		// if err == nil {
		// 	_, err = f.WriteString(nspeed.SeriesReport(wcWorkers))
		// }
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
		report := metric.GenerateJSONReport(&wcWorkers, *optUngroup)
		_, err = f.WriteString(report)
		if err != nil {
			log.Fatal().Msgf("could not write json report file (%s): %v", *optJSONReport, err)
		}
		f.Close()
	}

	// html report:
	if *optHTMLReport != "" {
		chartjs.GenerateHTMLReport(*optHTMLReport, wcWorkers, *optUngroup)
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
