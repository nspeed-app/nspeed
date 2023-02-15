package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"html/template"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"nspeed.app/nspeed"
)

var optTimeout = flag.Uint("timeout", 0, "default global timeout in seconds, 0 for no timeout")
var optTick = flag.Int("tick", 1, "sampling/progress interval in seconds (used for -debug, -cpu and webstats and filestats)")
var optCPU = flag.Bool("cpu", false, "display cpu usage")
var optColor = flag.Bool("color", false, "use colors in text output")

// banchmark commands, same as nspee cmd line arguments
// in template format to set N et Size
var testTemplates = map[string]string{
	"http1.1":  "server {{.Cmd}} -n {{.N}} http://localhost:7333/{{.Dir}}{{.Size}}",
	"http2c":   "server -h2c {{.Cmd}} -n {{.N}} -h2c http://localhost:7333/{{.Dir}}{{.Size}}",
	"https1.1": "server -self {{.Cmd}} -n {{.N}} -self -http1.1 https://localhost:7333/{{.Dir}}{{.Size}}",
	"https2":   "server -self {{.Cmd}} -n {{.N}} -self -http2 https://localhost:7333/{{.Dir}}{{.Size}}",
	"https3":   "server -self -http3 {{.Cmd}} -n {{.N}} -self -http3 https://localhost:7333/{{.Dir}}{{.Size}}",
}

func main() {

	flag.CommandLine.Usage = func() {
		name := "bench" //todo
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n\n", name)
		fmt.Fprintf(flag.CommandLine.Output(), "  %s [benchmark name] ... [benchmark name]\n\n", name)
		fmt.Fprintf(flag.CommandLine.Output(), "Available benchmarks:\n\n")
		for v, c := range testTemplates {
			fmt.Fprintf(flag.CommandLine.Output(), "  %s : %s\n", v, c)
		}
		fmt.Fprintf(flag.CommandLine.Output(), "\nother flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse() // parse global flags
	nspeed.OptTLSSelf = true

	formatLevel := func(i any) string {
		if i == nil {
			return ""
		}
		// wish we could call the default formatter but it's not exported
		// so we lose colors for level
		return strings.ToUpper(fmt.Sprintf("| %-6s|", i))
	}

	consoleLevel := zerolog.WarnLevel
	consoleWriter := nspeed.CreateFilteredWriter(
		zerolog.MultiLevelWriter(zerolog.ConsoleWriter{
			Out:         os.Stderr,
			NoColor:     false,
			FormatLevel: formatLevel,
		}),
		consoleLevel,
		false)
	log.Logger = log.Output(consoleWriter)

	log.Debug().Msgf("max cores = %d", nspeed.MaxParallelism())
	nspeed.GlobalOptions = nspeed.Options{
		ProgressInterval: 1,
		Timeout:          time.Duration(*optTimeout) * time.Second,
		Test:             false,
		TLSSelf:          true,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
		progressCPUTicker := time.NewTicker(time.Duration(*optTick) * time.Second) // todo: pass value
		progressCPUChannel = progressCPUTicker.C
	}

	values := map[string]string{
		"Cmd":  "get",
		"Dir":  "",
		"N":    "1",
		"Size": "40g",
	}
	for _, arg := range flag.Args() {
		ct := testTemplates[arg]
		if ct == "" {
			log.Fatal().Msgf("unknown test: '%s'", arg)
		}
		tpl := template.New("T")
		t := template.Must(tpl.Parse(ct))
		var c bytes.Buffer
		if err := t.Execute(&c, values); err != nil {
			log.Fatal().Err(err).Send()
		}
		cmd := strings.Fields(c.String())

		fmt.Println("runnning: ", cmd)
		jobs, err := nspeed.ParseCommands(cmd, nil, false)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		//
		rc := 0
		waitChan := make(chan error)
		var wcWorkers []*nspeed.Metrics = nil
		// RunJobs async
		go func() {
			var err error
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

		nspeed.GenerateTextReport(os.Stdout, wcWorkers, consoleLevel <= zerolog.InfoLevel)
	}
}
