package nspeed

import (
	"context"
	"net/http/httptrace"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type Job interface {
	GetTarget() string
	Expand([]Job) []Job
	GoString() string
	Start(context.Context, *WorkerMetrics) error
}

func ExpandJobs(jobs []Job) []Job {
	var out []Job
	for _, job := range jobs {
		out = job.Expand(out)
	}
	return out
}

// WorkerMetrics - main metrics struct
// implements to the io.Writer interface
// this needs heavy refactoring
type WorkerMetrics struct {
	Index             int
	Job               Job
	Done              bool // job ended
	Expired           bool // job timed out
	ResultChan        chan *WorkerMetrics
	Target            string
	DNSInfo           httptrace.DNSDoneInfo
	DNSInfoTime       time.Time
	ConnInfo          httptrace.GotConnInfo
	ConnInfoTime      time.Time
	ContentSize       int64
	TotalRead         int64
	TotalProcessed    int64
	StepSize          int64
	TestStartTime     time.Time
	ElapsedTime       time.Duration
	FirstResponseByte time.Time
	TLSDone           time.Time
	Verbose           bool
	mutex             sync.Mutex
}

func (wm *WorkerMetrics) Write(p []byte) (int, error) {
	n := len(p)
	wm.StepSize = int64(n)
	wm.TotalRead += wm.StepSize
	wm.ElapsedTime = time.Since(wm.TestStartTime)
	return n, nil
}

// PrintProgress prints the intermediary metrics
func (wm *WorkerMetrics) PrintProgress() {
	elapsedSecs := wm.ElapsedTime.Seconds()
	log.Info(
		"#", wm.Index+1, ";", elapsedSecs, ";",
		ByteCountBinary(wm.TotalRead, "B"), ";",
		ByteCountBinary(wm.ContentSize, "B"), ";", wm.StepSize, ";", wm.Target)
}

// PrintFinal prints the final metrics
func (wm *WorkerMetrics) PrintFinal() {
	log.Info(
		"#", wm.Index+1,
		" speed = ", FormatBitperSecond(wm.ElapsedTime.Seconds(), wm.TotalRead),
		" time = ", wm.ElapsedTime.Milliseconds(), " ms (", wm.ElapsedTime.Seconds(), " s)",
		" size = ", ByteCountDecimal(wm.TotalRead, "B"), " (", ByteCountBinary(wm.TotalRead, "iB"), ")",
		" url  = ", wm.Target)
}

// LogFinal prints the final metrics (structured format)
func (wm *WorkerMetrics) LogFinal() {
	log.WithFields(log.Fields{
		"speed": FormatBitperSecond(wm.ElapsedTime.Seconds(), wm.TotalRead),
		"time":  wm.ElapsedTime.Seconds(),
		"size":  wm.TotalRead,
		"url":   wm.Target,
	}).Info("#", wm.Index+1)

}
