package nspeed

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"nspeed.app/utils"
)

type Job interface {
	// per job implementation
	GetTarget() string
	GoString() string
	PrintString(bool) string
	IsInfinite() bool
	Init(*Metrics) error
	Run(context.Context, *sync.WaitGroup) error
	GetMetric() (NetMetric, error)
	//common, implemented here
	ID() string
	SetID(string)
	GroupID() string
	SetGroupID(string)
}

// JobData should be embeded in every struct implementing Job interface
// it provides ID/GID storage
type JobData struct {
	Job
	id    string
	group string
}

func (j *JobData) ID() string            { return j.id }
func (j *JobData) SetID(id string)       { j.id = id }
func (j *JobData) GroupID() string       { return j.group }
func (j *JobData) SetGroupID(gid string) { j.group = gid }

// a list of jobs, some having unique IDs stored in IDMap
type JobList struct {
	Jobs  []Job
	IDMap map[string]Job // this basically a cache to avoid looping Jobs
}

func NewJoblist() *JobList {
	return &JobList{
		IDMap: make(map[string]Job),
	}
}

// AddJobs adds jobs preventing duplicate id, adding and renaming groups
// if a job name is empty, generate a unique one
func (jl *JobList) AddJobs(jobs ...Job) error {
	id := "#" + strconv.Itoa(len(jl.Jobs))
	n := len(jobs)

	for i, j := range jobs {
		if j == nil {
			return fmt.Errorf("Addjob: nil argument")
		}
		if j.ID() == "" {
			j.SetID(id)
		}
		if n > 1 {
			// special, if job group, 'id' becomme 'id.n' but 'id' could already exists
			// this avoid 'id' and 'id.1', "id.2" with different commands
			j.SetGroupID(j.ID())
			if _, ok := jl.IDMap[j.ID()]; ok {
				return fmt.Errorf("duplicate job id %s", j.ID())
			}
			j.SetID(j.ID() + "." + strconv.Itoa(i+1))
		}
		_, ok := jl.IDMap[j.ID()]
		if ok {
			return fmt.Errorf("duplicate job id %s", j.ID())
		}
		jl.IDMap[j.ID()] = j
		jl.Jobs = append(jl.Jobs, j)
	}
	return nil
}

// MetricsData -
type MetricsData struct {
	Index              int
	Job                Job
	Done               bool // job ended
	Expired            bool // job timed out or other side ended
	Cancelled          bool
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
}

// Metrics main metrics struct
// implements the io.Writer and io.Reader interfaces
// this needs heavy refactoring
// & check openmetrics suff
type Metrics struct {
	MetricsData
	ResultChan chan *Metrics
	Logger     zerolog.Logger
	mutex      sync.Mutex
	Valid      bool
	Protocol   string
	Error      error
	// history
	Tick  time.Duration
	Serie []MetricsData
}

// UpdateData adds the lastest data to the history and also returns it
func (wm *Metrics) UpdateData() MetricsData {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()
	// still running, get metrics , store and return them
	if !wm.Valid && !wm.Done && !wm.Cancelled && !wm.Expired {
		d := wm.MetricsData
		// make this 'copy' the MetricsData
		wm.Serie = append(wm.Serie, d)
		return d
	}
	if len(wm.Serie) > 0 {
		return wm.Serie[len(wm.Serie)-1]
	}
	// no history , return 0
	return MetricsData{}
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
 -data : target (url, group, method, target/final ip:port, connections info, tls/crypto)
 -series: time, read, write, duration, counts, average buff size,
 -status: completed/time'd out
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
// ReadFrom version- performance sensitive, don't do much here
// it's basically a io.Discard with some metrics stored
func (wm *Metrics) FixMeReadFrom(r io.Reader) (n int64, err error) {
	wm.mutex.Lock()
	wm.TotalRead = 0
	wm.ReadCount = 0
	wm.StepSize = 0
	wm.TransfertStartTime = time.Now()
	wm.mutex.Unlock()
	for {
		read, err := r.Read(BigBuffer)
		s := int64(read)
		wm.mutex.Lock()
		wm.ReadCount++
		wm.TotalRead += s
		if wm.StepSize < s {
			wm.StepSize = s
		}
		wm.mutex.Unlock()
		if err != nil {
			break
		}
	}
	wm.ElapsedTime = time.Since(wm.TransfertStartTime)
	fmt.Printf("ReadFrom used with %d Read and %d max buff", wm.ReadCount, wm.StepSize)
	return wm.TotalRead, nil
}

// LogProgress logs the intermediary metrics
func (wm *Metrics) LogProgress(logger *zerolog.Logger) {
	if logger == nil {
		return
	}
	if wm.Done {
		log.Panic()
	}
	m := wm.MetricsData //critical section
	logger.Trace().
		Float64("Elapsed", m.ElapsedTime.Seconds()).
		Int64("TotalRead", m.TotalRead).
		Int64("ContentReadSize", m.ContentReadSize).
		Int64("TotalWrite", m.TotalWrite).
		Int64("ContentWriteSize", m.ContentWriteSize).
		Int64("stepsize", m.StepSize).
		Int64("ReadCount", m.ReadCount).
		Int64("WriteCount", m.WriteCount).
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
			Int64("Read", wm.TotalRead).
			Int64("RCount", wm.ReadCount).
			Int64("RSize", AverageReadSize).
			Int64("Write", wm.TotalWrite).
			Int64("WCount", wm.WriteCount).
			Int64("WSize", AverageWriteSize).
			Str("RBPS", utils.FormatBitperSecond(wm.TotalRead, wm.ElapsedTime)).
			Str("WBPS", utils.FormatBitperSecond(wm.TotalWrite, wm.ElapsedTime)).
			Float64("Dur", wm.ElapsedTime.Seconds()).
			Str("URL", wm.Target).
			Send()
	} else {
		wm.Logger.Info().Err(wm.Error).Send()
	}
}

func (wm *Metrics) PrintFinal(w io.Writer, verbose bool) {
	if wm.Valid {
		fmt.Fprintf(w, "%d:", wm.Index)
		fmt.Fprintf(w, "Read: %s ", utils.FormatBitperSecond(wm.TotalRead, wm.ElapsedTime))
		fmt.Fprintf(w, "Write: %s ", utils.FormatBitperSecond(wm.TotalWrite, wm.ElapsedTime))
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
	wm.Serie = nil
	wm.Logger.Debug().Msgf("wm InitNew called")
}

func (wm *Metrics) Close() error {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()
	wm.ElapsedTime = time.Since(wm.TransfertStartTime)
	wm.Logger.Debug().Msgf("Close() called after %v", wm.ElapsedTime)
	// assert wm.Done == false
	// we don't report 2+ calls to Close
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

// Implement io.Reader - (it's a 'Write' metrics since a PUT/POST will call this Read() here to send to http server)
// Read <- golang http client -> PUT/POST --> network --> server upload
func (wm *Metrics) Read(p []byte) (n int, err error) {
	n, err = wm.read(p)
	//fmt.Println("READ:", n, err, time.Since(wm.TransfertStartTime), wm.TotalWrite)
	return n, err
}
func (wm *Metrics) read(p []byte) (n int, err error) {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()

	// TODO: why this doesn't work ?
	// first read = start timer ?
	// if wm.TotalWrite == 0 {
	// 	wm.TransfertStartTime = time.Now()
	// 	wm.ElapsedTime = 0
	// } else {
	wm.ElapsedTime = time.Since(wm.TransfertStartTime)
	// }

	wm.WriteCount++
	maxstep := len(p)

	if maxstep == 0 {
		return 0, nil
	}

	lefttodo := wm.ContentWriteSize - wm.TotalWrite
	if lefttodo == 0 {
		return 0, io.EOF
	}

	todo := utils.Min(utils.Min(int64(maxstep), wm.StepSize), lefttodo)

	wm.TotalWrite += todo

	return int(todo), nil
}

// WriteTo - implement io.WriterTo interface - optimized Read
func (wm *Metrics) FixWriteTo(w io.Writer) (n int64, err error) {
	wm.mutex.Lock()
	wm.TransfertStartTime = time.Now()
	wm.TotalWrite = 0
	wm.StepSize = 0
	wm.WriteCount = 0
	wm.ElapsedTime = 0
	wm.mutex.Unlock()

	buflen := int64(len(BigBuffer))

	wm.Logger.Debug().Msgf("WriteTo loop starting for %d bytes (%d)\n", wm.TotalWrite, time.Since(wm.TransfertStartTime))
	for {
		todo := utils.Min(wm.ContentWriteSize-wm.TotalWrite, buflen)
		if todo <= 0 {
			break
		}
		n, err := w.Write(BigBuffer[:todo])
		written := int64(n)
		if err != nil {
			return wm.TotalWrite + written, err
		}
		wm.mutex.Lock()
		if written > wm.StepSize {
			wm.StepSize = written
		}
		wm.TotalWrite = wm.TotalWrite + written
		wm.WriteCount++
		wm.ElapsedTime = time.Since(wm.TransfertStartTime)
		wm.mutex.Unlock()
		if err != nil {
			break
		}
	}
	log.Debug().Msgf("WriteTo done: %d Write ops and %d max buff size. Total = %d bytes in %s\n", wm.WriteCount, wm.StepSize, wm.TotalWrite, wm.ElapsedTime)
	return wm.TotalWrite, nil
}

func initSpeedReport(w *tabwriter.Writer) {
	fmt.Fprintf(w, "Job\tId\tGid\tRead speed\tWrite speed\tTime\tBytes read\tBytes written\tcommand\n")
}
func speedReport(w *tabwriter.Writer, job string, id string, gid string, elapsedTime time.Duration, totalRead int64, totalWrite int64, target string) {
	fmt.Fprintf(w, "%s", fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%.2f\t%sB\t%sB\t%s\n",
		job, id, gid,
		utils.FormatBitperSecond(totalRead, elapsedTime),
		utils.FormatBitperSecond(totalWrite, elapsedTime),
		elapsedTime.Seconds(),
		utils.ByteCountDecimal(totalRead),
		utils.ByteCountDecimal(totalWrite),
		target))
}

// group subtotals
type reportGroup struct {
	ElapsedTime time.Duration
	TotalRead   int64
	TotalWrite  int64
}

// generate a tabular formatted text report
func GenerateJSONReport(wcWorkers []*Metrics) string {
	return "NO YET IMPLEMENTED\n"
}

func (wm *Metrics) SerieReport() (report string) {
	var ReadCount, WriteCount, TotalRead, TotalWrite int64
	var ElapsedTime time.Duration
	for i, w := range wm.Serie {
		if i == 0 {
			TotalRead = w.TotalRead
			ReadCount = w.ReadCount
			TotalWrite = w.TotalWrite
			WriteCount = w.WriteCount
			ElapsedTime = w.ElapsedTime
		} else {
			TotalRead = w.TotalRead - TotalRead
			ReadCount = w.ReadCount - ReadCount
			TotalWrite = w.TotalWrite - TotalWrite
			WriteCount = w.WriteCount - WriteCount
			ElapsedTime = w.ElapsedTime - ElapsedTime
		}
		report += fmt.Sprintf("serie:%s %d %d %d %d %s %s %s\n",
			wm.Job.ID(),
			TotalRead,
			ReadCount,
			TotalWrite,
			WriteCount,
			utils.FormatBitperSecond(TotalRead, ElapsedTime),
			utils.FormatBitperSecond(TotalWrite, ElapsedTime),
			ElapsedTime)
		report += fmt.Sprintf("cumul: %s %d %d %d %d %s\n",
			w.Job.ID(),
			w.TotalRead,
			w.ReadCount,
			w.TotalWrite,
			w.WriteCount,
			w.ElapsedTime)
	}
	report += fmt.Sprintf("total: %s %d %d %d %d %s\n",
		wm.Job.ID(),
		wm.TotalRead,
		wm.ReadCount,
		wm.TotalWrite,
		wm.WriteCount,
		wm.ElapsedTime)
	return
}
func SeriesReport(wcWorkers []*Metrics) (report string) {
	for _, wm := range wcWorkers {
		report = report + wm.SerieReport() // fmt.Sprintf("%d - %s - has %d data to report\n", wm.Index, wm.Job.ID(), len(wm.Serie))

	}
	return
}

// generate a tabular formatted text report
func GenerateTextReport(wcWorkers []*Metrics, verbose bool) string {
	// main speed report - todo: refactor in a function

	var out bytes.Buffer

	var elapsedTime time.Duration = 0
	var totalRead int64 = 0
	var totalWrite int64 = 0
	groups := make(map[string]*reportGroup)

	w := tabwriter.NewWriter(&out, 0, 2, 1, ' ', tabwriter.AlignRight|tabwriter.Debug)
	initSpeedReport(w)

	for _, wm := range wcWorkers {
		if wm.Valid {
			totalRead += wm.TotalRead
			totalWrite += wm.TotalWrite
			if wm.ElapsedTime > elapsedTime {
				elapsedTime = wm.ElapsedTime
			}
			speedReport(w, fmt.Sprintf(" %d", wm.Index), wm.Job.ID(), wm.Job.GroupID(), wm.ElapsedTime, wm.TotalRead, wm.TotalWrite, wm.Job.PrintString(verbose))
			if wm.Job.GroupID() != "" {
				g, ok := groups[wm.Job.GroupID()]
				if !ok {
					g = &reportGroup{}
					groups[wm.Job.GroupID()] = g
				}
				g.TotalRead += wm.TotalRead
				g.TotalWrite += wm.TotalWrite
				if wm.ElapsedTime > g.ElapsedTime {
					g.ElapsedTime = wm.ElapsedTime
				}
			}
		} else {
			if wm.Error != nil {
				log.Warn().Err(wm.Error).Msgf("job %d ended", wm.Index)
			}
		}
	}
	for gid, g := range groups {
		speedReport(w, "Subtotal", "", gid, g.ElapsedTime, g.TotalRead, g.TotalWrite, "")
	}

	if totalRead == 0 && totalWrite == 0 {
		fmt.Println("no metrics to report")
	} else {
		speedReport(w, "Total", "", "", elapsedTime, totalRead, totalWrite, "")
		w.Flush()
	}
	return out.String()
}

func GenerateHTMLReport(out io.Writer, wcWorkers []*Metrics) {
	//todo:
}

// normalized metrics:

// one test summary:
// target/description
// group id
// date/time
// protocol, version, etc (http, crypto)
// endpoints: ip,ip, hardware specs?
// download_speed_bps	Download speed (bit/s)
// upload_speed_bps	Upload speed (bit/s)
// ping_ms	Latency (ms)
// bytes_received	Bytes received during test
// bytes_sent	Bytes sent during test

// one test every tick:
// tick id (index value starting from 0 or 1)
// tick_timestamp absolute datetime ?
// bytes_received	Bytes received since last tick
// bytes_sent	Bytes sent since last tick
// latency (optionnal)

/* Job -> Run -> final metrics
   Job -> Tick(..) -> real time metrics

   -store metrics in each job or outside ?
     -> format all metrics in same data struct or use interfaces ?
*/

type NetMetric struct {
	Description    string        `json:"description"`
	Group          int           `json:"group"`
	Date           time.Time     `json:"date"`
	RXBytes        uint64        `json:"rxbytes"`
	TXBytes        uint64        `json:"txbytes"`
	AverageLatency uint64        `json:"latency"`
	Duration       time.Duration `json:"duration"`
	Samples        []NetMetricSample
}

type NetMetricSample struct {
	Offset   time.Duration `json:"index"`
	RXBytes  uint64        `json:"rxbytes"`
	TXBytes  uint64        `json:"txbytes"`
	Latency  uint64        `json:"latency"`
	Duration uint64        `json:"duration"` // in ms or ns ? - should be constant - keep?
}

/*  echarts.apache.org

option = {
  xAxis: {
    type: 'value', // or time?
    data: [1,2,3,4,5,6,7,8]
  },
  yAxis: {
    type: 'value'
  },
  series: [
    {
      data: [150, 230, 224, 218, 135, 147, 260, 500],
      type: 'line'
    }
  ]
};
*/
