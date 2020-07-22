package nspeed

import (
	"fmt"
	"net/http/httptrace"
	"time"
)

// WorkerMetrics - main metrics struct
// implements to the io.Writer interface
type WorkerMetrics struct {
	Index             int
	Done              bool
	ResultChan        chan *WorkerMetrics
	URL               string
	DNSInfo           httptrace.DNSDoneInfo
	DNSInfoTime       time.Time
	ConnInfo          httptrace.GotConnInfo
	ConnInfoTime      time.Time
	ContentSize       int64
	TotalRead         int64
	StepSize          int64
	StartTime         time.Time
	ElapsedTime       time.Duration
	FirstResponseByte time.Time
}

func (wm *WorkerMetrics) Write(p []byte) (int, error) {
	n := len(p)
	wm.StepSize = int64(n)
	wm.TotalRead += wm.StepSize
	wm.ElapsedTime = time.Since(wm.StartTime)
	return n, nil
}

// PrintProgress prints the intermediary metrics
func (wm *WorkerMetrics) PrintProgress() {
	elapsedSecs := wm.ElapsedTime.Seconds()
	fmt.Println(wm.Index+1, ";", elapsedSecs, ";",
		ByteCountBinary(wm.TotalRead, "B"), ";",
		ByteCountBinary(wm.ContentSize, "B"), ";", wm.StepSize, ";", wm.URL)
}

// PrintFinal prints the final metrics
func (wm *WorkerMetrics) PrintFinal() {
	fmt.Println(
		wm.Index+1,
		"speed =", FormatBitperSecond(wm.ElapsedTime.Seconds(), wm.TotalRead),
		"time =", wm.ElapsedTime,
		"size =", wm.TotalRead, ByteCountDecimal(wm.TotalRead, "B"),
		"url ", wm.URL)
}
