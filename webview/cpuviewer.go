package webview

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/shirou/gopsutil/v3/cpu"
)

const (
	//
	VCPU = "CPU"
)

// CPUViewer collects the cpu number metrics via `gopsutil/cpu`
type CPUViewer struct {
	smgr  *StatsMgr
	graph *charts.Line
}

// NewCPUViewer returns the CPUViewer instance
// Series: CPUCores
func NewCPUViewer(tick time.Duration, path string) Viewer {
	graph := newBasicView(tick, VCPU, path)
	graph.SetGlobalOptions(
		charts.WithYAxisOpts(opts.YAxis{
			Name:        "CPU Load",
			Scale:       opts.Bool(false),
			Min:         0,
			SplitNumber: 10,
			Max:         100,
			GridIndex:   0,
			AxisLabel:   &opts.AxisLabel{Formatter: "{value} %"},
		}),
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(false)}),
	)
	cores, _ := cpu.Percent(0, true)
	for idx := range cores {
		graph.AddSeries(fmt.Sprintf("Core %d", idx), []opts.LineData{})
	}

	return &CPUViewer{graph: graph}
}

func (vr *CPUViewer) SetStatsMgr(smgr *StatsMgr) {
	vr.smgr = smgr
}

func (vr *CPUViewer) Name() string {
	return VCPU
}

func (vr *CPUViewer) View() *charts.Line {
	return vr.graph
}

func (vr *CPUViewer) Serve(w http.ResponseWriter, _ *http.Request) {
	vr.smgr.Tick()

	pcts, _ := cpu.Percent(0, true)
	for idx := range pcts {
		pcts[idx] = math.Round(pcts[idx])
	}
	metrics := Metrics{
		Values: pcts,
		Time:   time.Now().Format(defaultCfg.TimeFormat),
	}
	bs, _ := json.Marshal(metrics)
	_, _ = w.Write(bs)
}
