package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"

	"./nspeed"
	"github.com/shirou/gopsutil/cpu"
)

// options / arguments
var optCpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
var optMemprofile = flag.String("memprofile", "", "write memory profile to file")
var optVerbose = flag.Bool("verbose", false, "show progress & details")
var optTimeout = flag.Int("timeout", 8, "timeout in seconds, 0 for no timeout")
var optTick = flag.Int("tick", 1, "progress interval in seconds only used in verbose mode, default 1")

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

	fmt.Println("max cores = ", nspeed.MaxParallelism())

	var wg sync.WaitGroup
	wg.Add(flag.NArg())
	results := make(chan *nspeed.WorkerMetrics, flag.NArg())

	wcWorkers := make([]*nspeed.WorkerMetrics, flag.NArg())

	for idx := range flag.Args() {
		wm := nspeed.WorkerMetrics{Done: false, Index: idx, URL: flag.Args()[idx], ResultChan: results}
		wcWorkers[idx] = &wm
		// todo check valid url, prefetch, do dns if needed, etc
	}

	// we could merge this loop with the previous one but in case we want to start sync everything we keep this separate
	for _, wm := range wcWorkers {
		fmt.Println("spawning workload ", wm.Index, wm.URL, wm.Done)
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
			// warning for backlogs if what's done here takes too long
			pcts, _ := cpu.Percent(0, true)
			for i := range pcts {
				fmt.Printf("%02.0f ", pcts[i])
			}
			fmt.Println("tick ", rc, runtime.NumGoroutine())
			for _, wm := range wcWorkers {
				if !wm.Done {
					wm.PrintProgress()
				}
			}
		case wm := <-results:
			rc++
			if wm.Done {
				fmt.Println("Child ended - ", wm.Index, runtime.NumGoroutine())
			}
		case <-mastertimeout:
			fmt.Println("master timeout reached")
			notdone = false
			//clean end children
		case <-cWaitgroupEnded:
			fmt.Println("all workers done")
			notdone = false
		}
	}
	fmt.Println("main loop ended", "rc =", rc)

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
		"summary:",
		"speed =", nspeed.FormatBitperSecond(elapsedTime.Seconds(), totalRead),
		"time =", elapsedTime,
		"size =", totalRead, nspeed.ByteCountDecimal(totalRead, "B"))

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
