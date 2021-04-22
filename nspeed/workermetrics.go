package nspeed

import (
	"context"
	"io"
	"math/rand"
	"net/http/httptrace"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type Job interface {
	GetTarget() string
	Expand([]Job) []Job
	GoString() string
	Start(context.Context, *WorkerMetrics) error // todo: split this into Init & Run
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
// & check openmetrics suff
type WorkerMetrics struct {
	Index              int
	Job                Job
	Done               bool // job ended
	Expired            bool // job timed out or other side ended
	ResultChan         chan *WorkerMetrics
	Target             string
	DNSInfo            httptrace.DNSDoneInfo
	DNSInfoTime        time.Time
	ConnInfo           httptrace.GotConnInfo
	ConnInfoTime       time.Time
	ContentSize        int64
	TotalRead          int64
	TotalProcessed     int64
	StepSize           int64
	TransfertStartTime time.Time
	ElapsedTime        time.Duration
	FirstResponseByte  time.Time
	TLSDone            time.Time
	LogLevel           log.Level
	mutex              sync.Mutex
	Valid              bool
	Error              error
}

// Writer
func (wm *WorkerMetrics) Write(p []byte) (int, error) {
	n := len(p)
	wm.StepSize = int64(n)
	wm.TotalRead += wm.StepSize
	wm.ElapsedTime = time.Since(wm.TransfertStartTime)
	return n, nil
}

// PrintProgress prints the intermediary metrics
func (wm *WorkerMetrics) PrintProgress() {
	if wm.LogLevel >= log.InfoLevel {
		log.Infof("job %d | elapsed %.2f secs | %sB of %sB | buffer = %d",
			wm.Index, wm.ElapsedTime.Seconds(),
			ByteCountBinary(wm.TotalRead), ByteCountBinary(wm.ContentSize),
			wm.StepSize)
	}
}

// PrintFinal prints the final metrics
func (wm *WorkerMetrics) PrintFinal() {
	if wm.Valid {
		log.Info(
			"#", wm.Index,
			" speed = ", FormatBitperSecond(wm.ElapsedTime.Seconds(), wm.TotalRead),
			" time = ", wm.ElapsedTime.Milliseconds(), " ms (", wm.ElapsedTime.Seconds(), " s)",
			" size = ", ByteCountDecimal(wm.TotalRead)+"B", " (", ByteCountBinary(wm.TotalRead)+"B", ")",
			" url  = ", wm.Target)
		return
	}
	log.Info("#", wm.Index, " error: ", wm.Error)
}

// LogFinal prints the final metrics (structured format)
func (wm *WorkerMetrics) LogFinal() {
	log.WithFields(log.Fields{
		"speed": FormatBitperSecond(wm.ElapsedTime.Seconds(), wm.TotalRead),
		"time":  wm.ElapsedTime.Seconds(),
		"size":  wm.TotalRead,
		"url":   wm.Target,
	}).Info("#", wm.Index)

}

// initialize metrics
func (wm *WorkerMetrics) InitNew() {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()

	wm.Valid = false
	wm.TransfertStartTime = time.Now()
	wm.ElapsedTime = 0
	wm.TotalRead = 0
}

func (wm *WorkerMetrics) Close() error {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()
	wm.ElapsedTime = time.Since(wm.TransfertStartTime)
	// assert wm.Done == false
	wm.Done = true
	if wm.TotalProcessed < wm.ContentSize {
		wm.Expired = true
	}
	wm.Valid = true
	return nil
}

// not in std lib...
func min(x, y int64) int64 {
	if x < y {
		return x
	}
	return y
}

// implement reader -
// first iteration - do nothing with the buffer
func (wm *WorkerMetrics) Read(p []byte) (n int, err error) {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()

	maxstep := len(p)

	if maxstep == 0 {
		return 0, nil
	}
	lefttodo := wm.ContentSize - wm.TotalProcessed
	if lefttodo == 0 {
		return 0, io.EOF
	}

	todo := min(min(int64(maxstep), wm.StepSize), lefttodo)

	wm.TotalRead += todo
	wm.TotalProcessed += todo

	wm.ElapsedTime = time.Since(wm.TransfertStartTime)
	return int(todo), nil
}

// init a buffer of random values
func InitChunk(chunkSize int64, seed int64) []byte {
	rng := rand.New(rand.NewSource(seed))
	// prepare a buffer of chunkSize bytes (todo: move this out of here to not stress GC)
	var chunk []byte
	for i := int64(0); i < chunkSize; i++ {
		chunk = append(chunk, byte(rng.Intn(256)))
	}
	return chunk
}
