package metric

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"nspeed.app/utils"
)

const gPrecision = 100 * time.Millisecond

// MetricsData - (We will replace this partially)
// type MetricsData struct { ... } // remove or comment previous struct

// Metric main metrics struct
type Metric struct {
	// Removed MetricsData, now handled by OTel instruments and attributes
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
	Tick            time.Duration
	Serie           []NetMetricSample // keeping the samples for now
	// logger
	logger *zerolog.Logger
	// OpenTelemetry
	meter             metric.Meter
	readBytes         metric.Int64Counter
	writeBytes        metric.Int64Counter
	readCount         metric.Int64Counter
	writeCount        metric.Int64Counter
	duration          metric.Float64Histogram
	readStepSize      metric.Int64Histogram
	writeStepSize     metric.Int64Histogram
	loop              metric.Int64Counter
	loopMax           metric.Int64Counter
	avgLatency        metric.Int64Histogram
	transferStartTime time.Time
	elapsedTime       time.Duration
	firstResponseByte time.Time
	id                string
	groupID           string
	batch             int
	batchName         string
	command           string
	done              bool
	expired           bool
	cancelled         bool
	target            string
	contentReadSize   int64
	contentWriteSize  int64
	remote            netip.AddrPort
}

// NewMetric creates a new Metric instance.
func NewMetric(id string, groupID string, batch int, batchName string, command string, target string, contentReadSize int64, contentWriteSize int64, remote netip.AddrPort, instance int) *Metric {
	meter := otel.Meter("nspeed/metric") // Change "your_app_name" to your application's name
	readBytes, _ := meter.Int64Counter("metric.read.bytes", metric.WithDescription("Total bytes read"))
	writeBytes, _ := meter.Int64Counter("metric.write.bytes", metric.WithDescription("Total bytes written"))
	readCount, _ := meter.Int64Counter("metric.read.count", metric.WithDescription("Total read count"))
	writeCount, _ := meter.Int64Counter("metric.write.count", metric.WithDescription("Total write count"))
	duration, _ := meter.Float64Histogram("metric.transfer.duration", metric.WithDescription("Transfer duration"), metric.WithUnit("s"))
	readStepSize, _ := meter.Int64Histogram("metric.read.stepsize", metric.WithDescription("Read step size"), metric.WithUnit("bytes"))
	writeStepSize, _ := meter.Int64Histogram("metric.write.stepsize", metric.WithDescription("Write step size"), metric.WithUnit("bytes"))
	loop, _ := meter.Int64Counter("metric.loop", metric.WithDescription("Current loop"))
	loopMax, _ := meter.Int64Counter("metric.loop.max", metric.WithDescription("Max loop"))
	avgLatency, _ := meter.Int64Histogram("metric.latency.avg", metric.WithDescription("Average Latency"), metric.WithUnit("ms"))
	return &Metric{
		meter:            meter,
		readBytes:        readBytes,
		writeBytes:       writeBytes,
		readCount:        readCount,
		writeCount:       writeCount,
		duration:         duration,
		readStepSize:     readStepSize,
		writeStepSize:    writeStepSize,
		loop:             loop,
		loopMax:          loopMax,
		avgLatency:       avgLatency,
		id:               id,
		groupID:          groupID,
		batch:            batch,
		batchName:        batchName,
		command:          command,
		target:           target,
		contentReadSize:  contentReadSize,
		contentWriteSize: contentWriteSize,
		remote:           remote,
		Instance:         instance,
	}
}

// ... (rest of your existing code)

func (wm *Metric) SetTLSInfo(conn net.Conn) {
	if tlsConn, ok := conn.(*tls.Conn); ok {
		cs := tlsConn.ConnectionState()
		wm.Cipher = utils.TLSCipherSuite(&cs)
		wm.TLSVersion = utils.TLSVersionFriendlyName(cs.Version)
		wm.Protocol = "IP"
		if _, ok := tlsConn.NetConn().(*net.TCPConn); ok {
			wm.Protocol = "TCP"
		}
		if _, ok := tlsConn.NetConn().(*net.UDPConn); ok {
			wm.Protocol = "UDP"
		}
	}
}

// ... (SetLogger, Logger, StartTicker remain mostly unchanged)

// UpdateData adds the lastest data to the history and also returns it
func (wm *Metric) UpdateData() NetMetricSample {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()

	now := time.Now()
	elapsed := now.Sub(wm.transferStartTime)
	if !wm.Valid && !wm.done && !wm.cancelled && !wm.expired {
		data := NetMetricSample{
			Time:     now,
			Duration: elapsed.Seconds(),
		}
		wm.Serie = append(wm.Serie, data)

		return data
	}
	if len(wm.Serie) > 0 {
		return wm.Serie[len(wm.Serie)-1]
	}
	return NetMetricSample{}
}

// Writer - performance sensitive, don't do much here
func (wm *Metric) Write(p []byte) (int, error) {
	n := int(wm.WriteN(int64(len(p))))
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (wm *Metric) WriteN(n int64) int64 {
	// Attributes are defined each time
	attrs := []attribute.KeyValue{
		attribute.String("command", wm.command),
		attribute.String("target", wm.target),
		attribute.String("id", wm.id),
		attribute.String("group_id", wm.groupID),
		attribute.Int("batch", wm.batch),
		attribute.String("batch_name", wm.batchName),
	}

	// record data
	wm.elapsedTime = time.Since(wm.transferStartTime)
	if n > 0 {
		wm.writeBytes.Add(utils.Ctx(), n, metric.WithAttributes(attrs...))
	}

	wm.mutex.Lock()
	defer wm.mutex.Unlock()
	if n > wm.contentWriteSize {
		wm.writeStepSize.Record(utils.Ctx(), n, metric.WithAttributes(attrs...))
	}
	wm.elapsedTime = time.Since(wm.transferStartTime)
	if wm.contentWriteSize > 0 {
		// wm.TotalWrite += min(n, wm.contentWriteSize-wm.TotalWrite)
	} else {
		// wm.TotalWrite += n
	}
	wm.writeCount.Add(utils.Ctx(), 1, metric.WithAttributes(attrs...))
	return n
}

// ... (rest of your methods adapted to OpenTelemetry)

// ReadFrom version
func (wm *Metric) FixMeReadFrom(r io.Reader) (n int64, err error) {
	// record data
	attrs := []attribute.KeyValue{
		attribute.String("command", wm.command),
		attribute.String("target", wm.target),
		attribute.String("id", wm.id),
		attribute.String("group_id", wm.groupID),
		attribute.Int("batch", wm.batch),
		attribute.String("batch_name", wm.batchName),
	}
	wm.mutex.Lock()
	// wm.TotalRead = 0
	// wm.ReadCount = 0
	// wm.ReadStepSize = 0
	wm.transferStartTime = time.Now()
	wm.mutex.Unlock()
	for {
		read, err := r.Read(utils.BigBuffer)
		s := int64(read)
		wm.mutex.Lock()
		// wm.ReadCount++
		// wm.TotalRead += s
		if s > 0 {
			wm.readBytes.Add(utils.Ctx(), s, metric.WithAttributes(attrs...))
		}
		wm.readCount.Add(utils.Ctx(), 1, metric.WithAttributes(attrs...))
		wm.readStepSize.Record(utils.Ctx(), s, metric.WithAttributes(attrs...))

		// if wm.ReadStepSize < s {
		// 	wm.ReadStepSize = s
		// }
		wm.mutex.Unlock()
		if err != nil {
			break
		}
	}
	wm.elapsedTime = time.Since(wm.transferStartTime)
	fmt.Printf("ReadFrom used with %d Read and %d max buff", wm.readCount, wm.readStepSize)
	return wm.elapsedTime.Nanoseconds(), nil
}

//... (and so on)

// LogProgress logs the intermediary metrics
func (wm *Metric) LogProgress(logger *zerolog.Logger) {
	if logger == nil {
		return
	}
	if wm.done {
		log.Panic()
	}
	// MetricsData -> NetMetricSample
	m := wm.UpdateData() //critical section
	logger.Trace().
		Float64("Elapsed", m.Duration).
		Send()
}
func (wm *Metric) InitNew() {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()

	wm.Valid = false
	wm.transferStartTime = time.Now()
	wm.elapsedTime = 0
	// wm.TotalRead = 0
	// wm.TotalWrite = 0
	// wm.ReadCount = 0
	// wm.WriteCount = 0
	wm.Serie = nil
	wm.Logger().Debug().Msgf("wm InitNew called")
}
func (wm *Metric) ReadN(n int64) (int64, error) {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()
	wm.elapsedTime = time.Since(wm.transferStartTime)
	wm.writeCount.Add(utils.Ctx(), 1, metric.WithAttributes(
		attribute.String("command", wm.command),
		attribute.String("target", wm.target),
		attribute.String("id", wm.id),
		attribute.String("group_id", wm.groupID),
		attribute.Int("batch", wm.batch),
		attribute.String("batch_name", wm.batchName),
	))
	if n == 0 {
		return 0, nil
	}

	lefttodo := wm.contentWriteSize
	if lefttodo <= 0 {
		return 0, io.EOF
	}

	todo := min(n, wm.contentWriteSize, lefttodo)

	return todo, nil
}

// Close marks a Metric as valid and set various other statuses
func (wm *Metric) Close() error {
	wm.mutex.Lock()
	defer wm.mutex.Unlock()
	wm.elapsedTime = time.Since(wm.transferStartTime)
	wm.Logger().Debug().Msgf("Close() called after %v", wm.elapsedTime)
	wm.done = true

	wm.duration.Record(utils.Ctx(), wm.elapsedTime.Seconds(), metric.WithAttributes(
		attribute.String("command", wm.command),
		attribute.String("target", wm.target),
		attribute.String("id", wm.id),
		attribute.String("group_id", wm.groupID),
		attribute.Int("batch", wm.batch),
		attribute.String("batch_name", wm.batchName),
		attribute.Bool("expired", wm.expired),
		attribute.Bool("cancelled", wm.cancelled),
		attribute.Bool("done", wm.done),
	))

	// if wm.TotalWrite < wm.ContentWriteSize {
	// 	wm.Expired = true
	// }
	// if wm.TotalRead < wm.ContentReadSize {
	// 	wm.Expired = true
	// }
	wm.Valid = true
	return nil
}
