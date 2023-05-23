package nspeed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"nspeed.app/utils"
	"nspeed.app/webview/statics/chartjs"
)

type Job interface {
	// per job implementation
	GetTarget() string
	GoString() string
	PrintString(bool, bool) string
	IsInfinite() bool
	Init() error
	Run(context.Context, *sync.WaitGroup) (*Metric, error)
	//common, implemented here
	Logger() *zerolog.Logger
	SetLogger(*zerolog.Logger)
	ID() string
	SetID(string)
	GroupID() string
	SetGroupID(string)
	Batch() int
	SetBatch(batch int)
}

// JobData should be embeded in every struct implementing Job interface
// it provides ID/GID storage
type JobData struct {
	Job
	id     string
	group  string
	batch  int
	logger *zerolog.Logger
}

func (j *JobData) ID() string            { return j.id }
func (j *JobData) SetID(id string)       { j.id = id }
func (j *JobData) GroupID() string       { return j.group }
func (j *JobData) SetGroupID(gid string) { j.group = gid }
func (j *JobData) Batch() int            { return j.batch }
func (j *JobData) SetBatch(batch int)    { j.batch = batch }

// Logger returns a dedicated Logger or the default one
func (j *JobData) Logger() *zerolog.Logger {
	if j.logger == nil {
		return &log.Logger
	}
	return j.logger
}
func (j *JobData) SetLogger(logger *zerolog.Logger) {
	if j.logger != nil && logger != nil {
		j.logger.Info().Msgf("overwriting a logger")
	}
	newlogger := logger.With().Str("jobid", j.ID()).Logger()
	j.logger = &newlogger
}

type Batch struct {
	Name      string
	PreDelay  time.Duration
	PostDelay time.Duration
}

// a list of jobs
type JobList struct {
	Jobs              map[string]Job
	Groups            map[string][]Job
	Batches           []*Batch
	currentBatchEmpty bool
}

func NewJoblist() *JobList {
	return &JobList{
		Jobs:              make(map[string]Job),
		Groups:            make(map[string][]Job),
		Batches:           make([]*Batch, 0, 1),
		currentBatchEmpty: false,
	}
}

// AddBatch adds a new batch
//
//	store batches by index
//	check for unique names
//	discard empty ones
func (jl *JobList) AddBatch(batch *Batch) error {
	if batch == nil {
		return fmt.Errorf("nil batch")
	}
	if batch.Name != "" {
		for _, b := range jl.Batches {
			if b.Name == batch.Name {
				return fmt.Errorf("batch named '%s' already exists", batch.Name)
			}
		}
	} else {
		batch.Name = "#" + strconv.Itoa(len(jl.Batches))
	}
	if !jl.currentBatchEmpty {
		jl.Batches = append(jl.Batches, batch)
	} else {
		jl.Batches[len(jl.Batches)-1] = batch
	}
	jl.currentBatchEmpty = true
	return nil
}

// AddJobs adds jobs as a group, preventing duplicate id/gid
// if a job name is empty, generate a unique one
// this is hard to read, rewrite properly
func (jl *JobList) AddJobs(jobs ...Job) error {
	if len(jl.Batches) == 0 {
		return fmt.Errorf("addjob but no batch yet")
	}
	batch := len(jl.Batches) - 1
	gid := "#" + strconv.Itoa(len(jl.Groups))
	n := len(jobs)

	var js []Job
	for i, j := range jobs {
		j.SetBatch(batch)
		jl.currentBatchEmpty = false
		if j == nil {
			return fmt.Errorf("func Addjob: nil argument")
		}
		if j.ID() == "" {
			j.SetID(gid)
		} else {
			gid = j.ID()
		}
		// group is ID for single job
		j.SetGroupID(gid)

		if n > 1 {
			j.SetID(j.ID() + "." + strconv.Itoa(i+1))
		}
		if _, ok := jl.Jobs[j.ID()]; ok {
			return fmt.Errorf("duplicate job id %s", j.ID())
		}
		jl.Jobs[j.ID()] = j
		js = append(js, j)
	}
	if _, ok := jl.Groups[gid]; ok {
		return fmt.Errorf("duplicate group id %s", gid)
	}
	jl.Groups[gid] = js
	return nil
}

// MetricsData -
type MetricsData struct {
	ID                 string
	GroupID            string
	Batch              int
	Command            string
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
	ReadStepSize       int64
	WriteStepSize      int64
	TransfertStartTime time.Time
	ElapsedTime        time.Duration
	FirstResponseByte  time.Time
	AvgLatency         time.Duration
	Remote             string
}

// Metric main metrics struct
// implements the io.Writer and io.Reader interfaces
// this needs heavy refactoring
// & check openmetrics suff
type Metric struct {
	MetricsData
	Logger     zerolog.Logger
	mutex      sync.Mutex
	Valid      bool
	Protocol   string
	Cipher     string
	TLSVersion string
	Error      error
	Instances  int
	Interface  *net.Interface
	SourceAddr netip.Addr
	Gateway    netip.Addr
	// history
	Tick  time.Duration
	Serie []MetricsData
}

// StartTicker starts a ticker for this Metric
func (wm *Metric) StartTicker(done chan any) {
	if wm.Tick <= 0 {
		return
	}
	ticker := time.NewTicker(wm.Tick)
	wm.Logger.Debug().Msgf("starting ticker at rate %s", wm.Tick)
	go func() {
		defer wm.Logger.Debug().Msg("go ticker ended")
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				wm.UpdateData()
			}
		}
	}()
}

// UpdateData adds the lastest data to the history and also returns it
func (wm *Metric) UpdateData() MetricsData {
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
func (wm *Metric) Write(p []byte) (int, error) {
	n := len(p)
	s := int64(n)
	wm.mutex.Lock()
	defer wm.mutex.Unlock()
	if s > wm.ReadStepSize {
		wm.ReadStepSize = s
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
func (wm *Metric) FixMeReadFrom(r io.Reader) (n int64, err error) {
	wm.mutex.Lock()
	wm.TotalRead = 0
	wm.ReadCount = 0
	wm.ReadStepSize = 0
	wm.TransfertStartTime = time.Now()
	wm.mutex.Unlock()
	for {
		read, err := r.Read(BigBuffer)
		s := int64(read)
		wm.mutex.Lock()
		wm.ReadCount++
		wm.TotalRead += s
		if wm.ReadStepSize < s {
			wm.ReadStepSize = s
		}
		wm.mutex.Unlock()
		if err != nil {
			break
		}
	}
	wm.ElapsedTime = time.Since(wm.TransfertStartTime)
	fmt.Printf("ReadFrom used with %d Read and %d max buff", wm.ReadCount, wm.ReadStepSize)
	return wm.TotalRead, nil
}

// LogProgress logs the intermediary metrics
func (wm *Metric) LogProgress(logger *zerolog.Logger) {
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
		Int64("ReadStepsize", m.ReadStepSize).
		Int64("ReadCount", m.ReadCount).
		Int64("WriteStepsize", m.WriteStepSize).
		Int64("WriteCount", m.WriteCount).
		Send()
}

// LogFinal logs the final metrics
func (wm *Metric) LogFinal(logger *zerolog.Logger) {
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

func (wm *Metric) PrintFinal(w io.Writer) {
	if wm.Valid {
		fmt.Fprintf(w, "Read: %s ", utils.FormatBitperSecond(wm.TotalRead, wm.ElapsedTime))
		fmt.Fprintf(w, "Write: %s ", utils.FormatBitperSecond(wm.TotalWrite, wm.ElapsedTime))
		fmt.Fprintf(w, "TotalRead: %d ", wm.TotalRead)
		fmt.Fprintf(w, "TotalWrite: %d ", wm.TotalWrite)
		// fmt.Fprintf(w, "ReadBPS: %d ", (int64)((float64)(wm.TotalRead)*8.0/wm.ElapsedTime.Seconds()))
		// fmt.Fprintf(w, "WriteBPS: %d ", (int64)((float64)(wm.TotalWrite)*8.0/wm.ElapsedTime.Seconds()))
		fmt.Fprintf(w, "duration: %s ", wm.ElapsedTime)
		fmt.Fprintf(w, "command: %s", "TODO1"+wm.Target)
		fmt.Fprintf(w, "\n")

	} else {
		fmt.Fprintf(w, "job %s error: %v\n", "TODO2", wm.Error)
	}
}

// InitNew (re)initializes a metric - todo rename
func (wm *Metric) InitNew() {
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

func (wm *Metric) Close() error {
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
func (wm *Metric) Read(p []byte) (n int, err error) {
	n, err = wm.read(p)
	//fmt.Println("READ:", n, err, time.Since(wm.TransfertStartTime), wm.TotalWrite)
	return n, err
}

// actual implementation of Read
func (wm *Metric) read(p []byte) (n int, err error) {
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

	todo := utils.Min(utils.Min(int64(maxstep), wm.WriteStepSize), lefttodo)

	wm.TotalWrite += todo

	return int(todo), nil
}

// WriteTo - implement io.WriterTo interface - optimized Read
func (wm *Metric) FixWriteTo(w io.Writer) (n int64, err error) {
	wm.mutex.Lock()
	wm.TransfertStartTime = time.Now()
	wm.TotalWrite = 0
	wm.WriteStepSize = 0
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
		if written > wm.WriteStepSize {
			wm.WriteStepSize = written
		}
		wm.TotalWrite = wm.TotalWrite + written
		wm.WriteCount++
		wm.ElapsedTime = time.Since(wm.TransfertStartTime)
		wm.mutex.Unlock()
		if err != nil {
			break
		}
	}
	log.Debug().Msgf("WriteTo done: %d Write ops and %d max buff size. Total = %d bytes in %s\n", wm.WriteCount, wm.WriteStepSize, wm.TotalWrite, wm.ElapsedTime)
	return wm.TotalWrite, nil
}

func initSpeedReport(w *tabwriter.Writer) {
	fmt.Fprintf(w, "Id\tRead speed\tWrite speed\tTime\tBytes read\tBytes written\tcommand\n")
}
func speedReport(w *tabwriter.Writer, id string, elapsedTime time.Duration, totalRead int64, totalWrite int64, target string) {
	fmt.Fprintf(w, "%s", fmt.Sprintf("%s\t%s\t%s\t%.2f\t%sB\t%sB\t%s\n",
		id,
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
	Command     string
	Instances   int
}

type Metrics []*Metric

// MarshalJSON for Workers type.
// if an entry has an error, it will be ignored
func (wc *Metrics) MarshalJSON() ([]byte, error) {
	todo := Metrics{}
	for _, v := range *wc {
		if v.Error == nil {
			todo = append(todo, v)
		}
	}
	return json.Marshal(todo)
}

// Clone deep copies a Metric
func (wm *Metric) Clone() *Metric {
	c := Metric{
		MetricsData: wm.MetricsData,
		Logger:      wm.Logger,
		Valid:       wm.Valid,
		Protocol:    wm.Protocol,
		Cipher:      wm.Cipher,
		TLSVersion:  wm.TLSVersion,
		Error:       wm.Error,
		Instances:   wm.Instances,
		Interface:   wm.Interface,
		SourceAddr:  wm.SourceAddr,
		Gateway:     wm.Gateway,
		Tick:        wm.Tick,
		Serie:       wm.Serie,
	}
	return &c
}

// Group replace instances of the groups with the sum of metrics
func (wc *Metrics) Group() *Metrics {
	if wc == nil {
		return nil
	}
	rwc := make(Metrics, 0, len(*wc))
	groups := make(map[string]*Metric)

	for _, wm := range *wc {
		if wm == nil {
			continue
		}
		if !wm.Valid {
			continue
		}
		if wm.Error != nil {
			continue
		}
		if wm.GroupID != wm.ID {
			g, ok := groups[wm.GroupID]
			if !ok {
				g = wm.Clone()
				g.Instances = 1
				groups[wm.GroupID] = g
				rwc = append(rwc, g)
				continue
			}
			g.TotalRead += wm.TotalRead
			g.TotalWrite += wm.TotalWrite
			g.ReadCount += wm.ReadCount
			g.ReadStepSize = -1
			g.WriteCount += wm.WriteCount
			g.WriteStepSize = -1
			g.Instances = g.Instances + 1
			if wm.ElapsedTime > g.ElapsedTime {
				g.ElapsedTime = wm.ElapsedTime
			}
		} else {
			rwc = append(rwc, wm)
		}
	}
	return &rwc
}

// generate a tabular formatted text report
func GenerateJSONReport(wcWorkers *Metrics, ungroup bool) string {
	wc := wcWorkers
	if !ungroup {
		wc = wcWorkers.Group()
	}
	json, err := json.MarshalIndent(wc, "", "\t")
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return string(json)
}

func (wm *Metric) MarshalJSON() ([]byte, error) {
	return json.Marshal(wm.ToNetMetric())
}

func (wm *Metric) ToNetMetric() NetMetric {
	nm := NetMetric{
		Command:    wm.Command,
		ID:         wm.ID,
		GroupID:    wm.GroupID,
		Batch:      wm.Batch,
		Streams:    uint64(wm.Instances),
		Date:       wm.TransfertStartTime,
		RXBytes:    uint64(wm.TotalRead),
		TXBytes:    uint64(wm.TotalWrite),
		Duration:   wm.ElapsedTime,
		RXHBps:     utils.FormatBitperSecond(wm.TotalRead, wm.ElapsedTime),
		TXHBps:     utils.FormatBitperSecond(wm.TotalWrite, wm.ElapsedTime),
		RXBps:      utils.BitperSecond(wm.TotalRead, wm.ElapsedTime),
		TXBps:      utils.BitperSecond(wm.TotalWrite, wm.ElapsedTime),
		RXChunk:    wm.ReadStepSize,
		TXChunk:    wm.WriteStepSize,
		RXCount:    wm.ReadCount,
		TXCount:    wm.WriteCount,
		Latency:    wm.AvgLatency,
		Remote:     wm.Remote,
		Protocol:   wm.Protocol,
		Cipher:     wm.Cipher,
		TLSVersion: wm.TLSVersion,
		SampleRate: uint64(wm.Tick),
		Samples:    wm.ToSamples(),
	}
	if wm.Interface != nil {
		nm.Interface = wm.Interface.Name
	}
	if wm.Gateway.IsValid() {
		nm.Gateway = wm.Gateway.String()
	}
	if wm.SourceAddr.IsValid() {
		nm.SrcAddr = wm.SourceAddr.String()
	}
	return nm
}

func (wm *Metric) ToSamples() []NetMetricSample {
	if wm.Serie == nil {
		return nil
	}
	nms := make([]NetMetricSample, len(wm.Serie))
	var prevElapsed time.Duration
	var nextTime = wm.TransfertStartTime
	for i, s := range wm.Serie {
		n := NetMetricSample{}
		if i == 0 {
			n.RXBytes = uint64(s.TotalRead)
			n.TXBytes = uint64(s.TotalWrite)
			n.Duration = s.ElapsedTime
			prevElapsed = s.ElapsedTime
		}
		if i > 0 {
			n.RXBytes = uint64(s.TotalRead) - nms[i-1].RXBytes
			n.TXBytes = uint64(s.TotalWrite) - nms[i-1].TXBytes
			n.Duration = s.ElapsedTime - prevElapsed
			prevElapsed = s.ElapsedTime
		}
		n.Time = nextTime
		nextTime = n.Time.Add(s.ElapsedTime)
		nms[i] = n
	}
	return nms
}

func (wm *Metric) SerieReport() (report string) {
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
			wm.ID,
			TotalRead,
			ReadCount,
			TotalWrite,
			WriteCount,
			utils.FormatBitperSecond(TotalRead, ElapsedTime),
			utils.FormatBitperSecond(TotalWrite, ElapsedTime),
			ElapsedTime)
	}
	report += fmt.Sprintf("total: %s %d %d %d %d %s\n",
		wm.ID,
		wm.TotalRead,
		wm.ReadCount,
		wm.TotalWrite,
		wm.WriteCount,
		wm.ElapsedTime)
	return
}
func SeriesReport(wcWorkers Metrics) (report string) {
	for _, wm := range wcWorkers {
		report = report + wm.SerieReport() // fmt.Sprintf("%d - %s - has %d data to report\n", wm.Index, wm.Job.ID(), len(wm.Serie))
	}
	return
}

// generate a report per batch (sorted), filtering failed jobs
func GenerateTextReport(jl *JobList, wcWorkers Metrics, showGroup bool) (out string) {
	if jl == nil {
		return "no job list"
	}
	batches := make(map[int]int, 1)
	for _, wm := range wcWorkers {
		if wm.Error != nil {
			out += fmt.Sprintf("batch %d, job %s, error = %s\n", wm.Batch, wm.ID, wm.Error)
			continue
		}
		if !wm.Valid {
			//out += fmt.Sprintf("batch %d, job %s, not valid\n", wm.Batch, wm.ID)
			continue
		}
		batches[wm.Batch]++
	}
	batchnums := make([]int, 0, len(batches))
	for k := range batches {
		batchnums = append(batchnums, k)
	}
	sort.Ints(batchnums)
	for _, batch := range batchnums {
		out += fmt.Sprintf("batch %s:\n", jl.Batches[batch].Name) + GenerateTextReportBatch(batch, wcWorkers, showGroup) + "\n"
	}
	return
}

// generate a tabular formatted text report
func GenerateTextReportBatch(batch int, wcWorkers Metrics, showGroup bool) string {
	// main speed report - todo: refactor in a function

	var out bytes.Buffer

	var elapsedTime time.Duration
	var totalRead int64
	var totalWrite int64
	groups := make(map[string]*reportGroup)

	w := tabwriter.NewWriter(&out, 0, 2, 1, ' ', tabwriter.AlignRight|tabwriter.Debug)
	initSpeedReport(w)

	for _, wm := range wcWorkers {
		if wm.Batch != batch {
			continue
		}
		if !wm.Valid {
			continue
		}
		if wm.Error != nil {
			log.Warn().Err(wm.Error).Msgf("job %s ended", wm.ID)
			continue
		}
		inGroup := false
		totalRead += wm.TotalRead
		totalWrite += wm.TotalWrite
		if wm.ElapsedTime > elapsedTime {
			elapsedTime = wm.ElapsedTime
		}
		if wm.GroupID != wm.ID {
			inGroup = true
			g, ok := groups[wm.GroupID]
			if !ok {
				g = &reportGroup{}
				groups[wm.GroupID] = g
				g.Command = wm.Command
			}
			g.Instances++
			g.TotalRead += wm.TotalRead
			g.TotalWrite += wm.TotalWrite
			if wm.ElapsedTime > g.ElapsedTime {
				g.ElapsedTime = wm.ElapsedTime
			}
		}
		if !inGroup || showGroup {
			speedReport(w, wm.GroupID, wm.ElapsedTime, wm.TotalRead, wm.TotalWrite, wm.Command)
		}

	}
	for gid, g := range groups {
		speedReport(w, gid, g.ElapsedTime, g.TotalRead, g.TotalWrite, strconv.Itoa(g.Instances)+" x "+g.Command)
	}
	fmt.Fprintf(w, "\t\t\t\t\t\t\n")
	if totalRead == 0 && totalWrite == 0 {
		fmt.Println("no metrics to report")
	} else {
		speedReport(w, "Average", elapsedTime, totalRead, totalWrite, "")
		w.Flush()
	}
	return out.String()
}

func GenerateHTMLReport(htmlFilename string, wcWorkers Metrics, ungroup bool) {
	var out *utils.SmartFile
	var err error
	if htmlFilename != ":" {
		out, err = utils.FileCreate(htmlFilename)
		if err != nil {
			log.Fatal().Msgf("could not create html file (%s): %v", htmlFilename, err)
		}
	}
	json := GenerateJSONReport(&wcWorkers, ungroup)
	html := chartjs.ChartJSON(json)
	if out != nil {
		html, err = utils.InlineScript(html, "")
		if err != nil {
			log.Fatal().Msgf("could inline html file (%s): %v", htmlFilename, err)
		}
		_, err = out.WriteString(html)
		if err != nil {
			log.Fatal().Msgf("could not write to html file (%s): %v", htmlFilename, err)
		}
		log.Info().Msgf("html generated to %s", htmlFilename)
	} else {
		err := utils.OpenHTML(html)
		if err != nil {
			log.Fatal().Msgf("could not open html: %v", err)
		}
	}
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

// missing: expected bytes (Content-Length when time'out)

type NetMetric struct {
	Command    string        `json:"command"`
	ID         string        `json:"id"`
	GroupID    string        `json:"gid"`
	Batch      int           `json:"batch"`
	Streams    uint64        `json:"streams"`
	Date       time.Time     `json:"date"`
	RXBytes    uint64        `json:"rxbytes"`
	TXBytes    uint64        `json:"txbytes"`
	Duration   time.Duration `json:"duration"`
	RXHBps     string        `json:"rxhbps"`
	TXHBps     string        `json:"ryhbps"`
	RXBps      int64         `json:"rxbps"`
	TXBps      int64         `json:"txbps"`
	RXChunk    int64         `json:"rxchunk"`
	TXChunk    int64         `json:"txchunk"`
	RXCount    int64         `json:"rxcount"`
	TXCount    int64         `json:"txcount"`
	Latency    time.Duration `json:"latency"`
	Remote     string        `json:"remote"`
	Protocol   string        `json:"protocol"`
	TLSVersion string        `json:"tlsversion"`
	Cipher     string        `json:"cipher"`
	Interface  string        `json:"interface"`
	Gateway    string        `json:"gateway"`
	SrcAddr    string        `json:"sourceaddr"`
	SampleRate uint64        `json:"samplerate"`
	Samples    []NetMetricSample
}

type NetMetricSample struct {
	RXBytes  uint64        `json:"rxbytes"`
	TXBytes  uint64        `json:"txbytes"`
	Latency  int64         `json:"latency"`
	Time     time.Time     `json:"date"`
	Duration time.Duration `json:"duration"` // in ms or ns ? - should be constant - keep?
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
