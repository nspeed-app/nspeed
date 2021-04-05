package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"strconv"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	log "github.com/sirupsen/logrus"
	"nspeed.app/nspeed"
)

// commands
const getCommand = "get"
const putCommand = "put"
const serveCommand = "server"

var implementedCommands = map[string]bool{
	getCommand:   true,
	putCommand:   false,
	serveCommand: true,
}

// options / arguments
var optCpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
var optMemprofile = flag.String("memprofile", "", "write memory profile to file")
var optVerbose = flag.Bool("verbose", false, "show progress & details")
var optTimeout = flag.Int("timeout", 8, "timeout in seconds, 0 for no timeout")
var optTick = flag.Int("tick", 1, "progress interval in seconds only used in verbose mode, default 1")

type job struct {
	command   string
	repeat    int
	IPversion int
	target    string
}

// flag parsing - thx to https://github.com/artyom
func parseJobs(rawArgs []string) ([]job, error) {
	var out []job
	args := rawArgs
	fmt.Printf("%s\n", args)

	for len(args) != 0 {
		command := args[0]
		args = args[1:]
		val, _ := implementedCommands[command]
		if !val {
			return nil, errors.New("unknown command " + command)
		}
		j := job{}
		j.command = command
		switch command {
		case getCommand:
			fs := flag.NewFlagSet(command, flag.ExitOnError)
			repeat := fs.Uint("n", 1, "number of times to process an item")
			useIPv4 := fs.Bool("4", false, "use IPv4")
			useIPv6 := fs.Bool("6", false, "use IPv6")
			if err := fs.Parse(args); err != nil {
				return nil, err
			}
			j.repeat = int(*repeat)

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
			j.target = args[0]
			args = args[1:]
			out = append(out, j)
		case putCommand:
			return nil, errors.New("unimplemented command")
		case serveCommand:
			fs := flag.NewFlagSet(command, flag.ExitOnError)
			host := &nspeed.NSpeedHost{}

			fs.StringVar(&host.Host, "h", "", "Hostname or ip address to listen on")
			fs.IntVar(&host.Port, "p", 7330, "Port to listen on")
			fs.Int64Var(&host.MaxBodySize, "s", nspeed.DefaultMaxBodySize, "Maximum size of response, in bytes")
			fs.DurationVar(&host.MaxDuration, "t", nspeed.DefaultMaxDuration, "Maximum duration a response may take")
			fs.StringVar(&host.TLSCertFile, "cert", "", "TLS certificate file")
			fs.StringVar(&host.TLSKeyFile, "key", "", "TLS Key file")

			if err := fs.Parse(args); err != nil {
				return nil, err
			}
			// todo check cert & key & set ServeTLS
			host.ServeTLS = false

			nspeed.NSpeedServerApp(host)
		default:
			return nil, errors.New("unknown command")
		}
	}

	return out, nil
}

func expandJobs(jobs []job) []job {
	var out []job
	for _, job := range jobs {
		for i := 0; i < job.repeat; i++ {
			out = append(out, job)
		}
	}
	return out
}

// main
func main() {

	flag.Parse() // parse global flags
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

	jobs = expandJobs(jobs)
	njobs := len(jobs)
	results := make(chan *nspeed.WorkerMetrics, njobs)

	wcWorkers := make([]*nspeed.WorkerMetrics, njobs)

	for idx, job := range jobs {
		wm := nspeed.WorkerMetrics{
			Done:       false,
			Index:      idx,
			URL:        job.target,
			ResultChan: results,
			Verbose:    *optVerbose}
		wcWorkers[idx] = &wm
		// todo check valid url, prefetch, do dns if needed, add context ? etc
	}
	for _, wm := range wcWorkers {
		fmt.Printf("%v\n", wm.URL)
	}

	// we could merge this loop with the previous one but in case we want to start sync everything we keep this separate
	var wg sync.WaitGroup
	wg.Add(njobs)
	for _, wm := range wcWorkers {
		log.Debug("spawning workload ", wm.Index, " ", wm.URL, " ", wm.Done)
		go func(w *nspeed.WorkerMetrics) {
			defer wg.Done()
			err := nspeed.HTTPDownload(w)
			if err != nil {
				fmt.Println("error for ", w.URL)
				return
			}
		}(wm)
	}

	// main thread loop
	// - wait for (partial) result from workers
	// - wait for progress tick and print results
	// - wait for all workers to end or optTimeout elapsed

	cWaitgroupEnded := make(chan struct{})
	go func() {
		defer close(cWaitgroupEnded)
		wg.Wait()
		log.Debug("wait group ended")
	}()

	rc := 0

	notdone := true
	mastertimeout := time.After(time.Duration(*optTimeout) * time.Second)
	progressTicker := time.NewTicker(time.Duration(*optTick) * time.Second)

	for notdone {
		select {
		case <-progressTicker.C:
			// warning for overlap/backlogs if what's done here takes too long
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
				log.Debug("Child ended - ", wm.Index, runtime.NumGoroutine())
			}
		case <-mastertimeout:
			log.Debug("master timeout reached")
			notdone = false
			//clean end children
		case <-cWaitgroupEnded:
			log.Debug("all workers done")
			notdone = false
		}
	}
	log.Debug("main loop ended", "rc =", rc)

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
