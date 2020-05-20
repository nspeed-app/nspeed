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

	"github.com/shirou/gopsutil/cpu"

	"./nspeed"
)

// options / arguments
var optCpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
var optMemprofile = flag.String("memprofile", "", "write memory profile to file")
var optVerbose = flag.Bool("verbose", false, "show progress & details")
var optTimeout = flag.Int("timeout", 8, "timeout in seconds, 0 for no timeout")

// WriteCounter counts the number of bytes written.
// implements to the io.Writer interface
type WriteCounter struct {
	Index             int
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
	elapsedSecs := int(time.Since(wc.startTime).Seconds())
	if elapsedSecs > wc.elapsedSecs {
		wc.elapsedSecs = elapsedSecs
		if *optVerbose {
			wc.printProgress()
		}
	}
	return n, nil
}

func (wc WriteCounter) printProgress() {

	pcts, _ := cpu.Percent(0, true)
	for i := range pcts {
		fmt.Printf("%02.0f ", pcts[i])
	}

	fmt.Println(wc.Index+1, ";", wc.elapsedSecs, ";",
		nspeed.ByteCountBinary(wc.totalRead, "B"), ";",
		nspeed.ByteCountBinary(wc.contentSize, "B"), ";", wc.stepSize, ";", wc.URL)
}

func (wc WriteCounter) printFinal() {
	fmt.Println(
		wc.Index+1,
		nspeed.ByteCountDecimal(wc.totalRead*8/wc.elapsedTime.Milliseconds()*1000, "bps"),
		"time =", wc.elapsedTime,
		"size =", wc.totalRead,
		"url ", wc.URL)
}

// DownloadFile will download a url to a local file. It's efficient because it will
// write as it downloads and not load the whole file into memory. We pass an io.TeeReader
// into Copy() to report progress on the download.
func DownloadFile(url string, idx int) (time.Duration, int64, error) {

	// Get the data
	req, err := http.NewRequest("GET", url, nil)

	// todo: CheckRedirect

	if err != nil {
		return 0, 0, err
	}

	//
	wc := &WriteCounter{}
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
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		log.Fatal(err)
	}

	protocol := resp.Request.Proto
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
	for idx := range flag.Args() {
		fileURL := flag.Args()[idx]
		fmt.Println(idx+1, fileURL)
		go func(f string, i int) {
			_, _, err := DownloadFile(f, i)
			if err != nil {
				fmt.Println("error for ", f)
				return
			}
			wg.Done()
		}(fileURL, idx)
	}
	wg.Wait()

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

	fmt.Println("Download Finished")
}
