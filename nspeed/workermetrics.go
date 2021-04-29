package nspeed

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http/httptrace"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/rs/zerolog"
)

type Job interface {
	GetTarget() string
	Expand([]Job) []Job
	GoString() string
	PrintString() string
	IsInfinite() bool
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
	Index               int
	Job                 Job
	Done                bool // job ended
	Expired             bool // job timed out or other side ended
	ResultChan          chan *WorkerMetrics
	Target              string
	DNSInfo             httptrace.DNSDoneInfo
	DNSInfoTime         time.Time
	ConnInfo            httptrace.GotConnInfo
	ConnInfoTime        time.Time
	ContentReadSize     int64
	ContentWriteSize    int64
	TotalRead           int64
	TotalWrite          int64
	TotalReadProcessed  int64
	TotalWriteProcessed int64
	StepSize            int64
	TransfertStartTime  time.Time
	ElapsedTime         time.Duration
	FirstResponseByte   time.Time
	TLSDone             time.Time
	Logger              zerolog.Logger
	LogLevel            zerolog.Level
	mutex               sync.Mutex
	Valid               bool
	Error               error
}

// Writer
func (wm *WorkerMetrics) Write(p []byte) (int, error) {
	n := len(p)
	wm.StepSize = int64(n)
	wm.TotalRead += wm.StepSize
	wm.ElapsedTime = time.Since(wm.TransfertStartTime)
	return n, nil
}

// LogProgress logs the intermediary metrics
func (wm *WorkerMetrics) LogProgress(logger *zerolog.Logger) {
	if logger == nil {
		return
	}
	if wm.LogLevel <= zerolog.InfoLevel {
		logger.Info().
			Float64("Elapsed", wm.ElapsedTime.Seconds()).
			Int64("TotalRead", wm.TotalRead).
			Int64("ContentReadSize", wm.ContentReadSize).
			Int64("TotalWrite", wm.TotalWrite).
			Int64("ContentWriteSize", wm.ContentWriteSize).
			Int64("stepsize", wm.StepSize).
			Send()

		// log.Infof("job %d | elapsed %.2f secs | %sB of %sB | %sB of %sB | buffer = %d",
		// 	wm.Index, wm.ElapsedTime.Seconds(),
		// 	ByteCountBinary(wm.TotalRead), ByteCountBinary(wm.ContentReadSize),
		// 	ByteCountBinary(wm.TotalWrite), ByteCountBinary(wm.ContentWriteSize),
		// 	wm.StepSize)
	}
}

// LogFinal logs the final metrics
func (wm *WorkerMetrics) LogFinal(logger *zerolog.Logger) {
	if logger == nil {
		return
	}
	if wm.Valid {
		logger.Info().
			Int64("TotalRead", wm.TotalRead).
			Int64("TotalWrite", wm.TotalWrite).
			Int64("ReadBPS", (int64)((float64)(wm.TotalRead)*8.0/wm.ElapsedTime.Seconds())).
			Int64("WriteBPS", (int64)((float64)(wm.TotalWrite)*8.0/wm.ElapsedTime.Seconds())).
			Float64("duration", wm.ElapsedTime.Seconds()).
			Str("url", wm.Target).
			Send()
	} else {
		wm.Logger.Info().Err(wm.Error).Send()
	}
}

func (wm *WorkerMetrics) PrintFinal() {
	if wm.Valid {
		fmt.Printf("Job %d:", wm.Index)
		fmt.Printf("TotalRead: %d ", wm.TotalRead)
		fmt.Printf("TotalWrite: %d ", wm.TotalWrite)
		fmt.Printf("ReadBPS: %d ", (int64)((float64)(wm.TotalRead)*8.0/wm.ElapsedTime.Seconds()))
		fmt.Printf("WriteBPS: %d ", (int64)((float64)(wm.TotalWrite)*8.0/wm.ElapsedTime.Seconds()))
		fmt.Printf("duration: %v ", wm.ElapsedTime.Seconds())
		fmt.Printf("url: %s", wm.Target)

	} else {
		fmt.Printf("job %d error: %v\n", wm.Index, wm.Error)
	}
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
	if wm.TotalWriteProcessed < wm.ContentWriteSize {
		wm.Expired = true
	}
	if wm.TotalReadProcessed < wm.ContentReadSize {
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

// implement reader - (ie 'Write' metrics since a POST will call Read() here to send to http server)
// first iteration - do nothing with the buffer
func (wm *WorkerMetrics) Read(p []byte) (n int, err error) {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()

	maxstep := len(p)

	if maxstep == 0 {
		return 0, nil
	}
	lefttodo := wm.ContentWriteSize - wm.TotalWriteProcessed
	if lefttodo == 0 {
		return 0, io.EOF
	}

	todo := min(min(int64(maxstep), wm.StepSize), lefttodo)

	wm.TotalWrite += todo
	wm.TotalWriteProcessed += todo

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

func InitSpeedReport(w *tabwriter.Writer) {
	fmt.Fprintf(w, "Job\tRead\tWrite\tTime\ttarget\n")
}
func SpeedReport(w *tabwriter.Writer, job string, elapsedTime time.Duration, totalRead int64, totalWrite int64, target string) {
	fmt.Fprintf(w, "%s", fmt.Sprintf("%s\t%s\t%s\t%.2f\t%s\n",
		job,
		FormatBitperSecond(elapsedTime.Seconds(), totalRead),
		FormatBitperSecond(elapsedTime.Seconds(), totalWrite),
		elapsedTime.Seconds(),
		// totalRead, ByteCountDecimal(totalRead),
		// totalWrite, ByteCountDecimal(totalWrite),
		target))
}
