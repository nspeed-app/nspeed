package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/cpu"
	log "github.com/sirupsen/logrus"
	"nspeed.app/nspeed"
)

// options / arguments
var optCpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
var optMemprofile = flag.String("memprofile", "", "write memory profile to file")
var optVerbose = flag.Bool("verbose", false, "show progress & details")
var optTimeout = flag.Int("timeout", 8, "timeout in seconds, 0 for no timeout")
var optTick = flag.Int("tick", 1, "progress interval in seconds only used in verbose mode, default 1")

type job struct {
	N     int
	IP    int
	Items []string
}

// flag parsing - thx to https://github.com/artyom
func parseFlags(rawArgs []string) ([]job, error) {
	var out []job
	args := rawArgs
	fmt.Printf("%s\n", args)

	for len(args) != 0 {
		j := job{}
		fs := flag.NewFlagSet("", flag.ExitOnError)
		fs.IntVar(&j.N, "n", 1, "number of times to process an item")
		useIPv4 := fs.Bool("4", false, "use IPv4")
		useIPv6 := fs.Bool("6", false, "use IPv6")
		if err := fs.Parse(args); err != nil {
			return nil, err
		}

		// force IPv4 or IPv6
		if *useIPv4 && *useIPv6 {
			fs.Usage()
			return nil, errors.New("specify IPv4 or IPv6 but not both")
		}
		if *useIPv4 {
			j.IP = 4
		}
		if *useIPv6 {
			j.IP = 6
		}

		args = args[:0]
		fmt.Printf("%s\n", args)
		for i, s := range fs.Args() {
			if strings.HasPrefix(s, "-") {
				args = append(args, fs.Args()[i:]...)
				break
			}
			j.Items = append(j.Items, s)
		}
		if len(j.Items) != 0 {
			out = append(out, j)
		}
	}
	return out, nil
}

// main
func main() {

	flag.Parse()

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

	var wg sync.WaitGroup
	wg.Add(flag.NArg())
	results := make(chan *nspeed.WorkerMetrics, flag.NArg())

	wcWorkers := make([]*nspeed.WorkerMetrics, flag.NArg())

	js, err := parseFlags(flag.Args())
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%+v\n", js)
	return

	for idx := range flag.Args() {
		wm := nspeed.WorkerMetrics{
			Done:       false,
			Index:      idx,
			URL:        flag.Args()[idx],
			ResultChan: results,
			Verbose:    *optVerbose}
		wcWorkers[idx] = &wm
		// todo check valid url, prefetch, do dns if needed, etc
	}

	// we could merge this loop with the previous one but in case we want to start sync everything we keep this separate
	for _, wm := range wcWorkers {
		log.Debug("spawning workload ", wm.Index, wm.URL, wm.Done)
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
