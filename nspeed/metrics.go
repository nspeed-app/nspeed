package nspeed

import (
	"context"
	"fmt"
	"io"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Job interface {
	GetTarget() string
	Expand([]Job, int) []Job
	GoString() string
	PrintString(bool) string
	IsInfinite() bool
	Init(*Metrics) error
	Run(context.Context) error
}

func ExpandJobs(jobs []Job) []Job {
	var out []Job
	for g, job := range jobs {
		out = job.Expand(out, g)
	}
	return out
}

// Metrics - main metrics struct
// implements to the io.Writer interface
// this needs heavy refactoring
// & check openmetrics suff
type Metrics struct {
	Index              int
	Job                Job
	Done               bool // job ended
	Expired            bool // job timed out or other side ended
	Cancelled          bool
	ResultChan         chan *Metrics
	Target             string
	ContentReadSize    int64
	ContentWriteSize   int64
	ReadCount          int64
	WriteCount         int64
	TotalRead          int64
	TotalWrite         int64
	StepSize           int64
	TransfertStartTime time.Time
	ElapsedTime        time.Duration
	FirstResponseByte  time.Time
	Logger             zerolog.Logger
	mutex              sync.Mutex
	Valid              bool
	Error              error
}

// serialized metrics
/*
CPU:
 -data: procinfo
 -series: time,core num, %(0-100)
INTERFACES:
 - name, idx, type, max speed, etc
 - rx bytes, tx bytes
 - rx pkts, tx pkts
 - others stats (errors, etc)
HTTP GET/PUT:
 -data : target (url, group, method, final ip, connections info, tls/crypto)
 -series: time, read, write, duration
HTTP SERVER:
 -data : request (url, method, remote ip, connections info, tls/crypto)
 -series: time, read, write, duration
 LATENCY:
 -data : target
 -series: time, latency (ms)
*/

// Writer - performance sensitive, don't do much here
func (wm *Metrics) Write(p []byte) (int, error) {
	n := len(p)
	s := int64(n)
	wm.mutex.Lock()
	defer wm.mutex.Unlock()
	if s > wm.StepSize {
		wm.StepSize = s
	}
	wm.ElapsedTime = time.Since(wm.TransfertStartTime)
	wm.TotalRead += s
	wm.ReadCount++
	return n, nil
}

// todo: since we io.Copy(WorkerMetrics, Resp.Body) and io.Copy can call a ReadFrom
// investigate if we can implement ReadFrom instead of Write
// func (wm *WorkerMetrics) ReadFrom(r io.Reader) (n int64, err error) { TODO }

// LogProgress logs the intermediary metrics
func (wm *Metrics) LogProgress(logger *zerolog.Logger) {
	if logger == nil {
		return
	}
	logger.Info().
		Float64("Elapsed", wm.ElapsedTime.Seconds()).
		Int64("TotalRead", wm.TotalRead).
		Int64("ContentReadSize", wm.ContentReadSize).
		Int64("TotalWrite", wm.TotalWrite).
		Int64("ContentWriteSize", wm.ContentWriteSize).
		Int64("stepsize", wm.StepSize).
		Int64("ReadCount", wm.ReadCount).
		Int64("WriteCount", wm.WriteCount).
		Send()
}

// LogFinal logs the final metrics
func (wm *Metrics) LogFinal(logger *zerolog.Logger) {
	if logger == nil {
		return
	}
	if wm.Valid {
		AverageReadSize := int64(0)
		if wm.ReadCount > 0 {
			AverageReadSize = wm.TotalRead / wm.ReadCount
		}
		AverageWriteSize := int64(0)
		if wm.WriteCount > 0 {
			AverageWriteSize = wm.TotalWrite / wm.WriteCount
		}

		logger.Info().
			Int64("TotalRead", wm.TotalRead).
			Int64("ReadCount", wm.ReadCount).
			Int64("AverageReadSize", AverageReadSize).
			Int64("TotalWrite", wm.TotalWrite).
			Int64("WriteCount", wm.WriteCount).
			Int64("AverageWriteSize", AverageWriteSize).
			Int64("ReadBPS", (int64)((float64)(wm.TotalRead)*8.0/wm.ElapsedTime.Seconds())).
			Int64("WriteBPS", (int64)((float64)(wm.TotalWrite)*8.0/wm.ElapsedTime.Seconds())).
			Float64("duration", wm.ElapsedTime.Seconds()).
			Str("url", wm.Target).
			Send()
	} else {
		wm.Logger.Info().Err(wm.Error).Send()
	}
}

func (wm *Metrics) PrintFinal(w io.Writer, verbose bool) {
	if wm.Valid {
		fmt.Fprintf(w, "%d:", wm.Index)
		fmt.Fprintf(w, "Read: %s ", FormatBitperSecond(wm.ElapsedTime.Seconds(), wm.TotalRead))
		fmt.Fprintf(w, "Write: %s ", FormatBitperSecond(wm.ElapsedTime.Seconds(), wm.TotalWrite))
		fmt.Fprintf(w, "TotalRead: %d ", wm.TotalRead)
		fmt.Fprintf(w, "TotalWrite: %d ", wm.TotalWrite)
		// fmt.Fprintf(w, "ReadBPS: %d ", (int64)((float64)(wm.TotalRead)*8.0/wm.ElapsedTime.Seconds()))
		// fmt.Fprintf(w, "WriteBPS: %d ", (int64)((float64)(wm.TotalWrite)*8.0/wm.ElapsedTime.Seconds()))
		fmt.Fprintf(w, "duration: %v ", wm.ElapsedTime.Seconds())
		fmt.Fprintf(w, "command: %s", wm.Job.PrintString(verbose))
		fmt.Fprintf(w, "\n")

	} else {
		fmt.Fprintf(w, "job %d error: %v\n", wm.Index, wm.Error)
	}
}

// initialize metrics
func (wm *Metrics) InitNew() {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()

	wm.Valid = false
	wm.TransfertStartTime = time.Now()
	wm.ElapsedTime = 0
	wm.TotalRead = 0
	wm.TotalWrite = 0
	wm.ReadCount = 0
	wm.WriteCount = 0
}

func (wm *Metrics) Close() error {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()
	wm.ElapsedTime = time.Since(wm.TransfertStartTime)
	// assert wm.Done == false
	wm.Done = true
	if wm.TotalWrite < wm.ContentWriteSize {
		wm.Expired = true
	}
	if wm.TotalRead < wm.ContentReadSize {
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

// Implement io.Reader - (it's a 'Write' metrics since a POST will call this Read() here to send to http server)
// Read <- golang http client -> POST --> network --> server upload
func (wm *Metrics) Read(p []byte) (n int, err error) {

	wm.mutex.Lock()
	defer wm.mutex.Unlock()
	wm.ElapsedTime = time.Since(wm.TransfertStartTime)

	wm.WriteCount++
	maxstep := len(p)

	if maxstep == 0 {
		return 0, nil
	}

	lefttodo := wm.ContentWriteSize - wm.TotalWrite
	if lefttodo == 0 {
		return 0, io.EOF
	}

	todo := min(min(int64(maxstep), wm.StepSize), lefttodo)

	wm.TotalWrite += todo

	return int(todo), nil
}

func initSpeedReport(w *tabwriter.Writer) {
	fmt.Fprintf(w, "Job\tRead speed\tWrite speed\tTime\tBytes read\tBytes written\tcommand\n")
}
func speedReport(w *tabwriter.Writer, job string, elapsedTime time.Duration, totalRead int64, totalWrite int64, target string) {
	fmt.Fprintf(w, "%s", fmt.Sprintf("%s\t%s\t%s\t%.2f\t%sB\t%sB\t%s\n",
		job,
		FormatBitperSecond(elapsedTime.Seconds(), totalRead),
		FormatBitperSecond(elapsedTime.Seconds(), totalWrite),
		elapsedTime.Seconds(),
		ByteCountDecimal(totalRead),
		ByteCountDecimal(totalWrite),
		target))
}

// generate a tabular formatted text report
func GenerateTextReport(out io.Writer, wcWorkers []*Metrics, verbose bool) {
	// main speed report - todo: refactor in a function
	var elapsedTime time.Duration = 0
	var totalRead int64 = 0
	var totalWrite int64 = 0

	w := tabwriter.NewWriter(out, 0, 2, 1, ' ', tabwriter.AlignRight|tabwriter.Debug)
	initSpeedReport(w)
	for _, wm := range wcWorkers {
		if wm.Valid {
			totalRead += wm.TotalRead
			totalWrite += wm.TotalWrite
			if wm.ElapsedTime > elapsedTime {
				elapsedTime = wm.ElapsedTime
			}
			speedReport(w, fmt.Sprintf("Job %d", wm.Index), wm.ElapsedTime, wm.TotalRead, wm.TotalWrite, wm.Job.PrintString(verbose))
		} else {
			log.Warn().Err(wm.Error).Msgf("job %d ended", wm.Index)
		}
	}
	if totalRead == 0 && totalWrite == 0 {
		fmt.Println("no metrics to report")
	} else {
		speedReport(w, "Total", elapsedTime, totalRead, totalWrite, "")
		w.Flush()
	}
}

func GenerateHTMLReport(out io.Writer, wcWorkers []*Metrics) {
	//todo:
}
