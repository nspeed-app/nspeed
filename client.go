package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptrace"
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

// WriteCounter counts the number of bytes written.
// implements to the io.Writer interface
type WriteCounter struct {
	Index             int
	Done              bool
	resultChan        chan *WriteCounter
	URL               string
	dnsInfo           httptrace.DNSDoneInfo
	dnsInfoTime       time.Time
	connInfo          httptrace.GotConnInfo
	connInfoTime      time.Time
	contentSize       int64
	totalRead         int64
	stepSize          int64
	startTime         time.Time
	elapsedTime       time.Duration
	FirstResponseByte time.Time
}

func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.stepSize = int64(n)
	wc.totalRead += wc.stepSize
	wc.elapsedTime = time.Since(wc.startTime)
	wc.resultChan <- wc
	return n, nil
}

func (wc *WriteCounter) printProgress() {
	elapsedSecs := wc.elapsedTime.Seconds()
	fmt.Println(wc.Index+1, ";", elapsedSecs, ";",
		nspeed.ByteCountBinary(wc.totalRead, "B"), ";",
		nspeed.ByteCountBinary(wc.contentSize, "B"), ";", wc.stepSize, ";", wc.URL)
}

func (wc *WriteCounter) printFinal() {
	fmt.Println(
		wc.Index+1,
		"speed =", nspeed.FormatBitperSecond(wc.elapsedTime.Seconds(), wc.totalRead),
		"time =", wc.elapsedTime,
		"size =", wc.totalRead, nspeed.ByteCountDecimal(wc.totalRead, "B"),
		"url ", wc.URL)
}

// HTTPDownload implements HTTP GET command
func HTTPDownload(wc *WriteCounter) error {

	// create http client
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: http.DefaultTransport,
	}

	// Get the data
	req, err := http.NewRequest("GET", wc.URL, nil)

	// add headers
	//req.Header.Add("x-nspeed-app-version", "1.0")
	//"accept: application/octet-stream"

	// todo: CheckRedirect

	if err != nil {
		return err
	}

	//

	trace := &httptrace.ClientTrace{
		DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
			wc.dnsInfoTime = time.Now()
			wc.dnsInfo = dnsInfo
		},
		GotConn: func(connInfo httptrace.GotConnInfo) {
			wc.connInfoTime = time.Now()
			wc.connInfo = connInfo
		},
		GotFirstResponseByte: func() {
			wc.FirstResponseByte = time.Now()
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	resp, err := client.Transport.RoundTrip(req)
	if err != nil {
		log.Fatal(err)
	}

	protocol := resp.Proto
	fmt.Println(wc.Index+1, "size", resp.ContentLength, "procotol", protocol, "local IP:port", wc.connInfo.Conn.LocalAddr(), "remote IP:port", wc.connInfo.Conn.RemoteAddr())

	wc.startTime = time.Now()
	wc.contentSize = resp.ContentLength

	// this is where the main transfert happens.
	//   iteration 1: we use io.Copy to 'null' (Discard) and a TeeReader to do progression
	if _, err = io.Copy(ioutil.Discard, io.TeeReader(resp.Body, wc)); err != nil {
		return err
	}
	wc.elapsedTime = time.Since(wc.startTime)
	wc.Done = true
	wc.resultChan <- wc

	return nil
}

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
	results := make(chan *WriteCounter, 1)

	wcWorkers := make([]*WriteCounter, flag.NArg())

	for idx := range flag.Args() {
		wc := WriteCounter{Done: false, Index: idx, URL: flag.Args()[idx], resultChan: results}
		wcWorkers[idx] = &wc
		// todo check valid url, prefetch, do dns if needed, etc
	}

	// we could merge this loop with the previous one but in case we want to start sync everything we keep this separate
	for _, wc := range wcWorkers {
		fmt.Println("spawning workload ", wc.Index, wc.URL, wc.Done)
		go func(w *WriteCounter) {
			defer wg.Done()
			err := HTTPDownload(w)
			if err != nil {
				fmt.Println("error for ", w.URL)
				return
			}
		}(wc)
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
			for _, wc := range wcWorkers {
				if !wc.Done {
					wc.printProgress()
				}
			}
		case wc := <-results:
			rc++
			if wc.Done {
				fmt.Println("Child ended - ", wc.Index, runtime.NumGoroutine())
			}
		case <-mastertimeout:
			fmt.Println("master timeout reached")
			notdone = false
			//clean end children
		case <-cWaitgroupEnded:
			fmt.Println("wait group done")
			notdone = false
		}
	}
	fmt.Println("main loop ended")
	var elapsedTime time.Duration = 0
	var totalRead int64 = 0

	for _, wc := range wcWorkers {
		wc.printFinal()
		totalRead += wc.totalRead
		if wc.elapsedTime > elapsedTime {
			elapsedTime = wc.elapsedTime
		}
		fmt.Println(
			"summary:",
			"speed =", nspeed.FormatBitperSecond(elapsedTime.Seconds(), totalRead),
			"time =", elapsedTime,
			"size =", totalRead, nspeed.ByteCountDecimal(totalRead, "B"))

	}

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
