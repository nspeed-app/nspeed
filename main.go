package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
)

// WriteCounter counts the number of bytes written to it. It implements to the io.Writer interface
// and we can pass this into io.TeeReader() which will report progress on each write cycle.
type WriteCounter struct {
	Total uint64
}

func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Total += uint64(n)
	wc.printProgress()
	return n, nil
}

func (wc WriteCounter) printProgress() {
	// Clear the line by using a character return to go back to the start and remove
	// the remaining characters by filling it with spaces
	fmt.Printf("\r%s", strings.Repeat(" ", 35))

	// Return again and print current status of download
	// We use the humanize package to print the bytes in a meaningful way (e.g. 10 MB)
	fmt.Printf("\rDownloading... %s complete", humanize.Bytes(wc.Total))
}

// DownloadFile will download a url to a local file. It's efficient because it will
// write as it downloads and not load the whole file into memory. We pass an io.TeeReader
// into Copy() to report progress on the download.
func DownloadFile(url string) error {

	// Get the data
	resp, err := http.Get(url)

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	start := time.Now()

	// Create our progress reporter and pass it to be used alongside our writer
	counter := &WriteCounter{}
	if _, err = io.Copy(ioutil.Discard, io.TeeReader(resp.Body, counter)); err != nil {
		return err
	}
	elapsed := time.Since(start)

	// The progress use the same line so print a new line once it's finished downloading
	fmt.Print("\n")
	fmt.Println("elapsed", elapsed)
	return nil
}

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")

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

	fileURL := flag.Args()[0]
	fmt.Println("Downloading ", fileURL)
	err := DownloadFile(fileURL)
	if err != nil {
		panic(err)
	}

	fmt.Println("Download Finished")
}
