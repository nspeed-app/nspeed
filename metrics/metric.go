package metrics

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"nspeed.app/crypto"
	"nspeed.app/generics"
	"nspeed.app/humanize"
	nsutl "nspeed.app/nspeed/utils"
)

const gPrecision = 100 * time.Millisecond

// MetricsData -
type MetricsData struct {
	ID                 string
	GroupID            string
	Batch              int
	BatchName          string
	Command            string
	Done               bool // job ended
	Expired            bool // job timed out or other side ended
	Cancelled          bool
	Target             string
	ContentReadSize    int64 // Counter
	ContentWriteSize   int64 // Counter
	ReadCount          int64 // Counter
	WriteCount         int64 // Counter
	TotalRead          int64 // Counter
	TotalWrite         int64 // Counter
	ReadStepSize       int64 // histo?
	WriteStepSize      int64 // histo?
	Loop               uint
	LoopMax            uint
	TransfertStartTime time.Time
	ElapsedTime        time.Duration
	FirstResponseByte  time.Time
	AvgLatency         time.Duration
	Remote             netip.AddrPort
}

// Metric main metrics struct
// implements the io.Writer and io.Reader interfaces
// this needs heavy refactoring
// & check openmetrics suff
type Metric struct {
	MetricsData
	mutex           sync.Mutex
	Valid           bool
	Protocol        string
	Cipher          string
	TLSVersion      string
	Error           error
	Instance        int
	Interface       *net.Interface
	SourceAddr      netip.AddrPort
	Gateway         netip.Addr
	RouteSourceAddr netip.Addr
	// optional time series
	Tick   time.Duration
	Series []MetricsData
	// logger
	logger *zerolog.Logger
}

func (wm *Metric) SetTLSInfo(conn net.Conn) {
	if tlsConn, ok := conn.(*tls.Conn); ok {
		cs := tlsConn.ConnectionState()
		wm.Cipher = crypto.TLSCipherSuite(&cs)
		wm.TLSVersion = crypto.TLSVersionFriendlyName(cs.Version)
		wm.Protocol = "IP"
		if _, ok := tlsConn.NetConn().(*net.TCPConn); ok {
			wm.Protocol = "TCP"
		}
		if _, ok := tlsConn.NetConn().(*net.UDPConn); ok {
			wm.Protocol = "UDP"
		}
	}
}

// set the logger
func (wm *Metric) SetLogger(logger *zerolog.Logger) {
	wm.logger = logger
}

// safe Logging, always works
func (wm *Metric) Logger() *zerolog.Logger {
	if wm.logger != nil {
		return wm.logger
	}
	return generics.Ptr(zerolog.Nop())
}

// StartTicker starts a ticker for this Metric
func (wm *Metric) StartTicker(done chan any) {
	if wm.Tick <= 0 {
		//fmt.Println("NO TICKER")
		return
	}
	ticker := time.NewTicker(wm.Tick)
	wm.Logger().Debug().Msgf("starting ticker at rate %s", wm.Tick)
	//fmt.Printf("TICKER ON for %s duration %d\n", wm.ID, wm.Tick)
	go func() {
		defer func() {
			//fmt.Printf("TICKER OFF for %s\n", wm.ID)
			wm.Logger().Debug().Msg("go ticker ended")
		}()
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
func (wm *Metric) UpdateData() {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()
	// still running, get metrics , store and return them
	if !wm.Valid && !wm.Done && !wm.Cancelled && !wm.Expired {
		if wm.Series != nil {
			if wm.Series[len(wm.Series)-1] == wm.MetricsData {
				// no change since last call
				return
			}
		}
		d := wm.MetricsData
		// make this 'copy' the MetricsData
		wm.Series = append(wm.Series, d)
	}
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
	n := int(wm.WriteN(int64(len(p))))
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
func (wm *Metric) WriteN(n int64) int64 {
	// wm.mutex.Lock()
	// defer wm.mutex.Unlock()
	if n > wm.ReadStepSize {
		wm.ReadStepSize = n
	}
	wm.ElapsedTime = time.Since(wm.TransfertStartTime)
	if wm.ContentReadSize > 0 {
		wm.TotalRead += min(n, wm.ContentReadSize-wm.TotalRead)
	} else {
		wm.TotalRead += n
	}
	wm.ReadCount++
	return n
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
		s := int64(1024 * 1024)
		wm.mutex.Lock()
		wm.ReadCount++
		wm.TotalRead += s
		if wm.ReadStepSize < s {
			wm.ReadStepSize = s
		}
		wm.mutex.Unlock()
		// if err != nil {
		// 	break
		// }
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
		Uint("Loop", m.Loop).
		Uint("LoopMax", m.LoopMax).
		Send()
}

func (wm *Metric) GetAverageReadSize() int64 {
	if wm.ReadCount > 0 {
		return wm.TotalRead / wm.ReadCount
	}
	return 0
}
func (wm *Metric) GetAverageWriteSize() int64 {
	if wm.WriteCount > 0 {
		return wm.TotalWrite / wm.WriteCount
	}
	return 0
}

// LogFinal logs the final metrics
func (wm *Metric) LogFinal(logger *zerolog.Logger) {
	if logger == nil {
		return
	}

	if wm.Valid {
		logger.Info().
			Int64("Read", wm.TotalRead).
			Int64("RCount", wm.ReadCount).
			Int64("RSize", wm.GetAverageReadSize()).
			Int64("Write", wm.TotalWrite).
			Int64("WCount", wm.WriteCount).
			Int64("WSize", wm.GetAverageWriteSize()).
			Str("RBPS", humanize.FormatBitperSecond(wm.TotalRead, wm.ElapsedTime)).
			Str("WBPS", humanize.FormatBitperSecond(wm.TotalWrite, wm.ElapsedTime)).
			Float64("Dur", wm.ElapsedTime.Seconds()).
			Uint("Loop", wm.Loop).
			Uint("LoopMax", wm.LoopMax).
			Str("URL", wm.Target).
			Send()
	} else {
		wm.Logger().Info().Err(wm.Error).Send()
	}
}

func (wm *Metric) PrintFinal(w io.Writer) {
	if wm.Valid {
		fmt.Fprintf(w, "job %s command: %s\n", wm.ID, wm.Command)
		fmt.Fprintf(w, "Read: %s \n", humanize.FormatBitperSecond(wm.TotalRead, wm.ElapsedTime))
		fmt.Fprintf(w, "Write: %s \n", humanize.FormatBitperSecond(wm.TotalWrite, wm.ElapsedTime))
		fmt.Fprintf(w, "TotalRead: %d (%sB)\n", wm.TotalRead, humanize.ByteCountBinary(wm.TotalRead))
		fmt.Fprintf(w, "TotalWrite: %d (%sB)\n", wm.TotalWrite, humanize.ByteCountBinary(wm.TotalWrite))
		fmt.Fprintf(w, "duration: %s (%d/%d)\n", wm.ElapsedTime, wm.Loop, wm.LoopMax)
		fmt.Fprintf(w, "\n")

	} else {
		fmt.Fprintf(w, "job %s not valid. error = %v\n", wm.ID, wm.Error)
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
	wm.Series = nil
	wm.Logger().Debug().Msgf("wm InitNew called")
}

// Close marks a Metric as valid and set various other statuses
func (wm *Metric) Close() error {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()
	if wm.ElapsedTime == 0 {
		wm.ElapsedTime = time.Since(wm.TransfertStartTime)
	}
	wm.Logger().Debug().Msgf("Close() called after %v", wm.ElapsedTime)
	// assert wm.Done == false
	// we don't report 2+ calls to Close
	wm.Done = true

	if wm.ContentWriteSize > 0 && (wm.TotalWrite < wm.ContentWriteSize) {
		wm.Expired = true
	}
	if wm.ContentReadSize > 0 && (wm.TotalRead < wm.ContentReadSize) {
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
	r, err := wm.ReadN(int64(len(p)))
	return int(r), err
}

// ReadN fake read n bytes of data. it return the n or less dependending on the remaing bytes.
// or io.EOF is no more data is left.
func (wm *Metric) ReadN(n int64) (int64, error) {

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

	if n == 0 {
		return 0, nil
	}

	lefttodo := wm.ContentWriteSize - wm.TotalWrite
	if lefttodo <= 0 {
		return 0, io.EOF
	}

	todo := min(n, wm.WriteStepSize, lefttodo)

	wm.TotalWrite += todo

	return todo, nil
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

	buflen := int64(1024) //int64(len(utils.BigBuffer))

	wm.Logger().Debug().Msgf("WriteTo loop starting for %d bytes (%d)\n", wm.TotalWrite, time.Since(wm.TransfertStartTime))
	for {
		todo := min(wm.ContentWriteSize-wm.TotalWrite, buflen)
		if todo <= 0 {
			break
		}
		n, err := 0, errors.New("NYI") //w.Write(utils.BigBuffer[:todo])
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
	}
	wm.Logger().Debug().Msgf("WriteTo done: %d Write ops and %d max buff size. Total = %d bytes in %s\n", wm.WriteCount, wm.WriteStepSize, wm.TotalWrite, wm.ElapsedTime)
	return wm.TotalWrite, nil
}

// group subtotals
type reportGroup struct {
	ElapsedTime time.Duration
	TotalRead   int64
	TotalWrite  int64
	Command     string
	Instances   int
}

// a group of Metric
// with associated batch names
type Metrics struct {
	Metrics []*Metric
	Batches []string
}

func NewMetrics() Metrics {
	return Metrics{
		make([]*Metric, 0),
		make([]string, 0),
	}
}

func (wc *Metrics) Append(m *Metric) {
	wc.Metrics = append(wc.Metrics, m)
}

// rebuild by removing non valid metric
// removing unused batch names
func (wc *Metrics) Cleanup() {
	if wc == nil {
		return
	}

	var metrics []*Metric
	batchesMap := make(map[int]string, 0)
	maxIdx := 0
	for _, v := range wc.Metrics {
		if !v.Valid {
			continue
		}
		metrics = append(metrics, v)
		if v.Batch < len(wc.Batches) {
			batchesMap[v.Batch] = wc.Batches[v.Batch]
		} else {
			batchesMap[v.Batch] = "#" + strconv.Itoa(v.Batch)
		}
		maxIdx = max(maxIdx, v.Batch)
	}
	log.Debug().Msgf("batchesMap: %+v", batchesMap)
	wc.Batches = make([]string, maxIdx+1)
	for k, v := range batchesMap {
		wc.Batches[k] = v
	}
	wc.Metrics = metrics
}

// return max index for batch
// func (wc Metrics) MaxBatchIndex(isValid bool) int {
// 	n := 0
// 	for _, v := range wc.Metrics {
// 		if !isValid || (isValid && v.Valid) {
// 			n = max(n, v.Batch)
// 		}
// 	}
// 	return n
// }

// func (wc Metrics) SortByBatch(batches map[int]string) Metrics {
// 	r := make([]Metrics, wc.MaxBatchIndex(true))
// 	for _, w := range wc.Metrics {
// 		if w == nil {
// 			log.Debug().Msgf("nil Metric found")
// 			continue
// 		}
// 		if w.Valid {
// 			if _, ok := batches[w.Batch]; !ok {
// 				log.Error().Msgf("unknown batch found in SortByBatch")
// 				return nil
// 			}
// 			// assert (w.Batch < )
// 			r[w.Batch] = append(r[w.Batch], w)
// 		}
// 	}
// 	rm := Metrics{}
// 	for _, b := range r {
// 		rm = append(rm, b...)
// 	}
// 	return rm
// }

// MarshalJSON for Workers type.
// if an entry has an error, it will be ignored
func (wc *Metrics) MarshalJSON() ([]byte, error) {
	todo := make([]*Metric, len(wc.Metrics))
	for _, v := range wc.Metrics {
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
		logger:      wm.logger,
		Valid:       wm.Valid,
		Protocol:    wm.Protocol,
		Cipher:      wm.Cipher,
		TLSVersion:  wm.TLSVersion,
		Error:       wm.Error,
		Instance:    wm.Instance,
		Interface:   wm.Interface,
		SourceAddr:  wm.SourceAddr,
		Gateway:     wm.Gateway,
		Tick:        wm.Tick,
		Series:      wm.Series,
	}
	return &c
}

// Group replace instances of the groups with the sum of metrics
// unused
func (wc *Metrics) Group() *Metrics {
	if wc == nil {
		return nil
	}
	rwc := NewMetrics()
	groups := make(map[string]*Metric)

	for _, wm := range wc.Metrics {
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
				g.Instance = 1
				groups[wm.GroupID] = g
				rwc.Metrics = append(rwc.Metrics, g)
				continue
			}
			g.TotalRead += wm.TotalRead
			g.TotalWrite += wm.TotalWrite
			g.ReadCount += wm.ReadCount
			g.ReadStepSize = -1
			g.WriteCount += wm.WriteCount
			g.WriteStepSize = -1
			g.Instance = g.Instance + 1
			if wm.ElapsedTime > g.ElapsedTime {
				g.ElapsedTime = wm.ElapsedTime
			}
		} else {
			rwc.Metrics = append(rwc.Metrics, wm)
		}
	}
	return &rwc
}

// generate a list json from metrics
func GenerateJSONReport(results *Metrics, tick time.Duration, command, fmtV4, fmtV6 string) (string, error) {
	if results == nil {
		return "", fmt.Errorf("no metrics")
	}
	report := Report{
		Version:    ReportVersion,
		SampleRate: uint64(tick),
		Command:    command,
		Results:    make([]NetMetric, 0, len(results.Metrics)),
	}
	for _, r := range results.Metrics {
		report.Results = append(report.Results, r.ToNetMetric(fmtV4, fmtV6))
	}
	js, err := json.MarshalIndent(report, "", "\t")
	if err != nil {
		return "", fmt.Errorf("error marshsalling to json: %w", err)
	}
	return string(js), nil
}

func (wm *Metric) MarshalJSON() ([]byte, error) {
	//log.Warn().Msgf("check me!")
	return json.Marshal(wm.ToNetMetric(nsutl.FormatV4full, nsutl.FormatV6Full))
}

func (wm *Metric) ToNetMetric(fmtV4, fmtV6 string) NetMetric {
	nm := NetMetric{
		Command:    wm.Command,
		ID:         wm.ID,
		GroupID:    wm.GroupID,
		Batch:      wm.Batch,
		BatchName:  wm.BatchName,
		Stream:     uint64(wm.Instance),
		Date:       wm.TransfertStartTime,
		RXBytes:    uint64(wm.TotalRead),
		TXBytes:    uint64(wm.TotalWrite),
		Duration:   wm.ElapsedTime,
		RXHBps:     humanize.FormatBitperSecond(wm.TotalRead, wm.ElapsedTime),
		TXHBps:     humanize.FormatBitperSecond(wm.TotalWrite, wm.ElapsedTime),
		RXBps:      humanize.BitPerSecondFromInt64(wm.TotalRead, wm.ElapsedTime),
		TXBps:      humanize.BitPerSecondFromInt64(wm.TotalWrite, wm.ElapsedTime),
		RXChunk:    wm.ReadStepSize,
		TXChunk:    wm.WriteStepSize,
		RXCount:    wm.ReadCount,
		TXCount:    wm.WriteCount,
		Latency:    wm.AvgLatency,
		Remote:     nsutl.AnonymizeNetIP(wm.Remote.Addr(), fmtV4, fmtV6) + ":" + nsutl.Uitoa(uint(wm.Remote.Port())),
		Protocol:   wm.Protocol,
		Cipher:     wm.Cipher,
		TLSVersion: wm.TLSVersion,
		SampleRate: uint64(wm.Tick),
		Loop:       wm.Loop,
		LoopMax:    wm.LoopMax,
		Samples:    wm.ToSamples(),
	}
	if wm.Interface != nil {
		nm.Interface = wm.Interface.Name
	}
	if wm.Gateway.IsValid() {
		nm.Gateway = nsutl.AnonymizeNetIP(wm.Gateway, fmtV4, fmtV6)
	}
	if wm.SourceAddr.IsValid() {
		nm.SrcAddr = nsutl.AnonymizeNetIP(wm.SourceAddr.Addr(), fmtV4, fmtV6) + ":" + nsutl.Uitoa(uint(wm.SourceAddr.Port()))
	}
	return nm
}

// ToSamples converts the serie to samples
// if no sampling or if rate was too big we still get the last/all sample
// so at least 1 entry
// todo: investigate if the last entry can be 0 (exact divsion between of total duration by rate)
func (wm *Metric) ToSamples() []NetMetricSample {
	nbSamples := 1
	if wm.Series != nil {
		nbSamples = len(wm.Series) + 1
	}

	// j, _ := json.MarshalIndent(wm.Series, "", "\t")
	// fmt.Println(string(j))

	nms := make([]NetMetricSample, 0, nbSamples)
	var prevElapsed time.Duration
	var rxSoFar, txSoFar uint64
	var nextTime = wm.TransfertStartTime
	if wm.Series != nil {
		for i, s := range wm.Series {
			n := NetMetricSample{}
			duration := s.ElapsedTime
			if i == 0 {
				n.RXBytes = uint64(s.TotalRead)
				n.TXBytes = uint64(s.TotalWrite)
			}
			if i > 0 {
				n.RXBytes = uint64(s.TotalRead) - rxSoFar
				n.TXBytes = uint64(s.TotalWrite) - txSoFar
				duration = s.ElapsedTime - prevElapsed
			}
			prevElapsed = s.ElapsedTime
			rxSoFar += n.RXBytes
			txSoFar += n.TXBytes
			n.Time = nextTime.Round(gPrecision)
			nextTime = n.Time.Add(duration)

			n.RXbps = humanize.BitPerSecondFromUInt64(n.RXBytes, duration)
			n.TXbps = humanize.BitPerSecondFromUInt64(n.TXBytes, duration)
			n.RXhbps = humanize.ByteCountDecimal(humanize.BitPerSecondFromUInt64(n.RXBytes, duration)) + "bps"
			n.TXhbps = humanize.ByteCountDecimal(humanize.BitPerSecondFromUInt64(n.TXBytes, duration)) + "bps"

			n.Duration = duration.Seconds()
			nms = append(nms, n)
		}
	}
	// todo: review this
	duration := wm.ElapsedTime - prevElapsed
	if duration != 0 && len(nms) > 0 {
		lastSample := &(nms[len(nms)-1])
		lastSample.Duration = duration.Seconds()
		lastSample.Time = nextTime.Round(gPrecision)
		lastSample.RXBytes = uint64(wm.TotalRead) - rxSoFar
		lastSample.TXBytes = uint64(wm.TotalWrite) - txSoFar
		lastSample.RXbps = humanize.BitPerSecondFromUInt64(lastSample.RXBytes, duration)
		lastSample.TXbps = humanize.BitPerSecondFromUInt64(lastSample.TXBytes, duration)
		lastSample.RXhbps = humanize.ByteCountDecimal(humanize.BitPerSecondFromUInt64(lastSample.RXBytes, duration)) + "bps"
		lastSample.TXhbps = humanize.ByteCountDecimal(humanize.BitPerSecondFromUInt64(lastSample.TXBytes, duration)) + "bps"
	}
	return nms
}

func (wm *Metric) SerieReport() (report string) {
	var ReadCount, WriteCount, TotalRead, TotalWrite int64
	var ElapsedTime time.Duration
	for i, w := range wm.Series {
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
			humanize.FormatBitperSecond(TotalRead, ElapsedTime),
			humanize.FormatBitperSecond(TotalWrite, ElapsedTime),
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
	for _, wm := range wcWorkers.Metrics {
		report = report + wm.SerieReport() // fmt.Sprintf("%d - %s - has %d data to report\n", wm.Index, wm.Job.ID(), len(wm.Serie))
	}
	return
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
	BatchName  string        `json:"batchname"`
	Stream     uint64        `json:"stream"`
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
	Loop       uint          `json:"loop"`
	LoopMax    uint          `json:"loopmax"`
	Samples    []NetMetricSample
}

type NetMetricSample struct {
	RXBytes  uint64    `json:"rxbytes"`
	TXBytes  uint64    `json:"txbytes"`
	Latency  int64     `json:"latency"`
	Time     time.Time `json:"date"`
	Duration float64   `json:"duration"` // in ms or ns ? - should be constant - keep?
	RXbps    int64     `json:"rxbps"`
	TXbps    int64     `json:"txbps"`
	RXhbps   string    `json:"rxhbps"`
	TXhbps   string    `json:"txhbps"`
	Loop     uint      `json:"loop"`
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

const ReportVersion = 1

type Report struct {
	Version    int    `json:"version"`
	SampleRate uint64 `json:"samplerate"`
	Command    string `json:"command"`
	Results    []NetMetric
}

func ReportFromJSON(data string) (Report, error) {
	report := Report{}
	err := json.Unmarshal([]byte(data), &report)
	if err != nil {
		return Report{}, fmt.Errorf("error parsing json: %w", err)
	}
	if report.Version != ReportVersion {
		return Report{}, fmt.Errorf("unknown report version: got %d, expected %d", report.Version, ReportVersion)
	}
	return report, err
}
