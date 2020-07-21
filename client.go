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
	resultChan        chan string
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
	elapsedSecs       int
	FirstResponseByte time.Time
}

func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.stepSize = int64(n)
	wc.totalRead += wc.stepSize
	wc.resultChan <- wc.printProgress()

	// elapsedSecs := int(time.Since(wc.startTime).Seconds())
	// if elapsedSecs > wc.elapsedSecs {
	// 	wc.elapsedSecs = elapsedSecs

	return n, nil
}

func (wc *WriteCounter) printProgress() string {

	// pcts, _ := cpu.Percent(0, true)
	// for i := range pcts {
	// 	fmt.Printf("%02.0f ", pcts[i])
	// }

	return fmt.Sprintln(wc.Index+1, ";", wc.elapsedSecs, ";",
		nspeed.ByteCountBinary(wc.totalRead, "B"), ";",
		nspeed.ByteCountBinary(wc.contentSize, "B"), ";", wc.stepSize, ";", wc.URL)
}

func (wc *WriteCounter) printFinal() {

	// nyi - fix me
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("recovered from divide by zero")
		}
	}()
	t := wc.elapsedTime.Seconds()
	speed := "(too fast)"
	if t > 0 {
		speed = nspeed.ByteCountDecimal((int64)((float64)(wc.totalRead)*8.0/t), "bps")
	}
	fmt.Println(
		wc.Index+1,
		"speed =", speed,
		"time =", wc.elapsedTime,
		"size =", wc.totalRead, nspeed.ByteCountDecimal(wc.totalRead, "B"),
		"url ", wc.URL)
}

// HTTPDownload implements HTTP GET command
func HTTPDownload(url string, idx int, r chan string) (time.Duration, int64, error) {

	// create http client
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: http.DefaultTransport,
	}

	// Get the data
	req, err := http.NewRequest("GET", url, nil)

	// add headers
	//req.Header.Add("x-nspeed-app-version", "1.0")
	//"accept: application/octet-stream"

	// todo: CheckRedirect

	if err != nil {
		return 0, 0, err
	}

	//
	wc := &WriteCounter{}
	wc.resultChan = r
	wc.Index = idx
	wc.URL = url

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
		return 0, wc.totalRead, err
	}
	wc.elapsedTime = time.Since(wc.startTime)

	// The progress use the same line so print a new line once it's finished downloading
	wc.printFinal()

	return wc.elapsedTime, wc.totalRead, nil
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
	wg.Add(len(flag.Args()))
	results := make(chan string)

	for idx := range flag.Args() {
		fileURL := flag.Args()[idx]
		fmt.Println(idx+1, fileURL)
		go func(f string, i int, r chan string) {
			defer wg.Done()
			_, _, err := HTTPDownload(f, i, r)
			if err != nil {
				fmt.Println("error for ", f)
				return
			}
		}(fileURL, idx, results)
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
		case <-results:
			rc++
			//fmt.Println("Child sent - ", s, runtime.NumGoroutine())
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
