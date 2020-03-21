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

	"github.com/dustin/go-humanize"
)

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
var memprofile = flag.String("memprofile", "", "write memory profile to file")
var beQuiet = flag.Bool("quiet", false, "don't show progress")

// WriteCounter counts the number of bytes written.
// implements to the io.Writer interface
type WriteCounter struct {
	Index       int
	URL         string
	dnsInfo     httptrace.DNSDoneInfo
	connInfo    httptrace.GotConnInfo
	contentSize int64
	totalRead   int64
	stepSize    int64
	startTime   time.Time
	elapsedTime time.Duration
	elapsedSecs int
}

func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.stepSize = int64(n)
	wc.totalRead += wc.stepSize
	elapsedSecs := int(time.Since(wc.startTime).Seconds())
	if elapsedSecs > wc.elapsedSecs {
		wc.elapsedSecs = elapsedSecs
		if !*beQuiet {
			wc.printProgress()
		}
	}
	return n, nil
}

func (wc WriteCounter) printProgress() {

	fmt.Println(wc.Index+1, ";", wc.elapsedSecs, ";", humanize.Bytes(uint64(wc.totalRead)), ";", humanize.Bytes(uint64(wc.contentSize)), ";", wc.stepSize, ";", wc.URL)
}

func (wc WriteCounter) printFinal() {
	fmt.Println("time =", wc.elapsedTime, "size =", wc.totalRead, "for ", wc.URL, "index", wc.Index+1, "bps: ", uint64(float64(wc.totalRead)/wc.elapsedTime.Seconds()*8))
}

// DownloadFile will download a url to a local file. It's efficient because it will
// write as it downloads and not load the whole file into memory. We pass an io.TeeReader
// into Copy() to report progress on the download.
func DownloadFile(url string, idx int) (time.Duration, int64, error) {

	// Get the data
	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		return 0, 0, err
	}

	//
	wc := &WriteCounter{}
	wc.Index = idx
	wc.URL = url

	trace := &httptrace.ClientTrace{
		DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
			wc.dnsInfo = dnsInfo
		},
		GotConn: func(connInfo httptrace.GotConnInfo) {
			wc.connInfo = connInfo
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		log.Fatal(err)
	}

	wc.contentSize = resp.ContentLength
	wc.startTime = time.Now()
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

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	var wg sync.WaitGroup
	wg.Add(len(flag.Args()))
	for idx := range flag.Args() {
		fileURL := flag.Args()[idx]
		fmt.Println("Spawning download of ", fileURL, "index", idx+1)
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

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
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
