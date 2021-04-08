package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	log "github.com/sirupsen/logrus"
	"nspeed.app/nspeed"
)

// Version can be set at link time to override debug.BuildInfo.Main.Version,
// which is "(devel)" when building from within the module. See
// golang.org/issue/29814 and golang.org/issue/29228.
var Version string

// commands
const getCommand = "get"
const putCommand = "put"
const serveCommand = "server"

// list of supported commands. Value is the set number. Commands in the same set can be mixed together.
var implementedCommands = map[string]int{
	getCommand:   1,
	putCommand:   1,
	serveCommand: 2,
}

// options / arguments
var optCpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
var optMemprofile = flag.String("memprofile", "", "write memory profile to file")
var optVerbose = flag.Bool("verbose", false, "show progress & details")
var optTick = flag.Int("tick", 1, "progress interval in seconds only used in verbose mode, default 1")
var versionFlag = flag.Bool("version", false, "display version")
var optTimeout = flag.Uint("timeout", 0, "default timeout in seconds, 0 for no timeout")
var optTest = flag.Bool("test", false, "just test arguments, do nothing")

// flag parsing - thx to https://github.com/artyom for hints
// make sure global args are parsed first
// refactor: parsing should be done per command type
func parseJobs(rawArgs []string) ([]nspeed.Job, error) {
	var out []nspeed.Job
	args := rawArgs
	//fmt.Printf("%s\n", args)
	cmdSet := 0

	for len(args) != 0 {
		command := args[0]
		args = args[1:]
		set, ok := implementedCommands[command]
		if !ok {
			return nil, errors.New("unknown command " + command)
		}
		if cmdSet == 0 {
			cmdSet = set
		}
		if cmdSet != set {
			return nil, errors.New("cannot mix these commands " + command)
		}
		switch command {
		case getCommand:
			fs := flag.NewFlagSet(command, flag.ContinueOnError)
			repeat := fs.Uint("n", 1, "number of times to process an item")
			useIPv4 := fs.Bool("4", false, "use IPv4")
			useIPv6 := fs.Bool("6", false, "use IPv6")
			timeout := fs.Uint("t", 0, "timeout")
			useragent := fs.String("u", "nspeed.app/"+Version, "user-agent")
			if err := fs.Parse(args); err != nil {
				return nil, err
			}

			j := nspeed.HTTPjob{
				Timeout:   time.Duration(*timeout) * time.Second,
				UserAgent: *useragent,
			}
			j.Command = command
			j.Repeat = int(*repeat)

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
			args = fs.Args()
			if len(args) == 0 {
				fs.Usage()
				return nil, errors.New("missing argument")
			}
			j.Target = args[0]
			args = args[1:]
			out = append(out, &j)
		case putCommand:
			return nil, errors.New("unimplemented command")
		case serveCommand:
			fs := flag.NewFlagSet(command, flag.ContinueOnError)
			host := &nspeed.NSpeedHost{}

			fs.StringVar(&host.Host, "h", "", "Hostname or ip address to listen on")
			fs.IntVar(&host.Port, "p", nspeed.DefaultPort, "Port to listen on")
			fs.Int64Var(&host.MaxBodySize, "s", nspeed.DefaultMaxBodySize, "Maximum size of response, in bytes")
			fs.DurationVar(&host.MaxDuration, "t", nspeed.DefaultMaxDuration, "Maximum duration a response may take")
			fs.StringVar(&host.TLSCertFile, "cert", "", "TLS certificate file")
			fs.StringVar(&host.TLSKeyFile, "key", "", "TLS Key file")

			if err := fs.Parse(args); err != nil {
				return nil, err
			}
			// todo check cert & key & set ServeTLS
			host.ServeTLS = false

			args = fs.Args()
			j := nspeed.NSpeedHTTPServer{Parameters: host}
			out = append(out, &j)
			// nspeed.NSpeedServerApp(host)

		default:
			return nil, errors.New("unknown command")
		}
	}

	return out, nil
}

// main
func main() {

	flag.Parse() // parse global flags

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

	jobs, err := parseJobs(flag.Args())
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%+v\n", jobs)
	if *optCpuprofile != "" {
		f, err := os.Create(*optCpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	log.SetLevel(log.InfoLevel)
	if *optVerbose {
		log.SetLevel(log.DebugLevel)
	}

	log.Debug("max cores = ", nspeed.MaxParallelism())

	jobs = nspeed.ExpandJobs(jobs)

	fmt.Println("Jobs:")
	for i, j := range jobs {
		fmt.Printf("  %d %#v\n", i, j)
	}

	njobs := len(jobs)
	results := make(chan *nspeed.WorkerMetrics, njobs)

	wcWorkers := make([]*nspeed.WorkerMetrics, njobs)

	for idx, job := range jobs {
		wm := nspeed.WorkerMetrics{
			Done:       false,
			Expired:    false,
			Index:      idx,
			Target:     job.GetTarget(),
			ResultChan: results,
			Verbose:    *optVerbose,
			Job:        job}
		wcWorkers[idx] = &wm
		// todo check valid url, prefetch, do dns if needed, add context ? etc
	}
	for _, wm := range wcWorkers {
		fmt.Printf("%v\n", wm.Target)
	}

	// we could merge this loop with the previous one but in case we want to start sync everything we keep this separate
	ctx := context.Background()
	if *optTimeout > 0 {
		newctx, cancel := context.WithTimeout(ctx, time.Duration(*optTimeout)*time.Second)
		defer cancel()
		ctx = newctx
	}

	var wg sync.WaitGroup
	wg.Add(njobs)
	for _, wm := range wcWorkers {
		log.Debug("spawning workload ", wm.Index, " ", wm.Target, " ", wm.Done)
		go func(w *nspeed.WorkerMetrics) {
			defer wg.Done()
			err := w.Job.Start(ctx, w)
			if err != nil {
				fmt.Println("error for ", w.Target)
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
		log.Debug("wait group ended")
	}()

	rc := 0

	notdone := true
	if *optTest {
		notdone = false
	}
	progressTicker := time.NewTicker(time.Duration(*optTick) * time.Second)

	for notdone {
		select {
		case <-progressTicker.C:
			// time critical: warning for overlap/backlogs if what's done here takes too long
			pcts, _ := cpu.Percent(0, true)
			if *optVerbose {
				f := log.Fields{}
				for idx := range pcts {
					f["core#"+strconv.Itoa(idx)] = pcts[idx]
				}
				log.WithFields(f).Debug("cpu ", rc, runtime.NumGoroutine())
			}
			for _, wm := range wcWorkers {
				if !wm.Done && wm.Verbose {
					wm.PrintProgress()
				}
			}
		case wm := <-results:
			rc++
			if wm.Done {
				log.Debugf("job %d ended - (on timeout: %t) - (%d remaining active goroutines)", wm.Index, wm.Expired, runtime.NumGoroutine())
			}
		case <-cWaitgroupEnded:
			log.Debugf("all workers done (%d remaining active goroutines)", runtime.NumGoroutine())
			notdone = false
		}
	}
	log.Debug("main loop ended", "rc = ", rc)

	var elapsedTime time.Duration = 0
	var totalRead int64 = 0

	for _, wm := range wcWorkers {
		wm.PrintFinal()
		totalRead += wm.TotalRead
		if wm.ElapsedTime > elapsedTime {
			elapsedTime = wm.ElapsedTime
		}
	}
	fmt.Println(
		"total:",
		"speed =", nspeed.FormatBitperSecond(elapsedTime.Seconds(), totalRead),
		"time =", elapsedTime,
		"size =", totalRead, " (", nspeed.ByteCountDecimal(totalRead, "B)"))

	// todo: defer this block
	if *optMemprofile != "" {
		f, err := os.Create(*optMemprofile)
		if err != nil {
			log.Fatal("could not create memory profile: ", err)
		}
		defer f.Close() // error handling omitted for example
		runtime.GC()    // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal("could not write memory profile: ", err)
		}
	}
}
