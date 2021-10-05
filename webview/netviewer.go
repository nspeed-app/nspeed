package webview

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
)

var ErrNotImplementedError = errors.New("not implemented yet")

const (
	//
	VNetInterfaces = "Network Interfaces"
)

var ChartNames = map[string]string{
	"rx": "download",
	"tx": "upload",
}

// NetInterfacesViewer collects network interfaces counters
type NetInterfacesViewer struct {
	smgr       *StatsMgr
	graph      *charts.Line
	direction  string
	interfaces []net.Interface
	lastvalues []float64
	lastread   time.Time
}

// NewNetInterfacesViewer returns the NetInterfacesViewer instance
// Series: net interfaces
func NewNetInterfacesViewer(tick time.Duration, dir string) Viewer {
	graph := newBasicView(tick, VNetInterfaces+dir)
	graph.SetGlobalOptions(
		charts.WithToolboxOpts(opts.Toolbox{
			Show: true,
			Feature: &opts.ToolBoxFeature{
				SaveAsImage: &opts.ToolBoxFeatureSaveAsImage{
					Show: true,
				},
				DataZoom: &opts.ToolBoxFeatureDataZoom{
					Show: true,
				},
				DataView: &opts.ToolBoxFeatureDataView{
					Show: true,
				},
			},
		}),
		charts.WithYAxisOpts(opts.YAxis{
			Name: ChartNames[dir],
			//AxisLabel: &opts.AxisLabel{Formatter: "function (value, index) { return formatBytes(value,index,2) }"},
			AxisLabel: &opts.AxisLabel{Formatter: opts.FuncOpts("function (value, index) { return formatBytes(value,'bps',2) }")},
		}),
		charts.WithLegendOpts(opts.Legend{Show: true}),
		charts.WithTooltipOpts(opts.Tooltip{
			Show:      true,
			Trigger:   "axis",
			Formatter: opts.FuncOpts("function (params) {return formatTooltip(params) }"),
		}),
	)
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Fatalf("error calling net.Interfaces(): %s", err)
	}
	var interfaces []net.Interface
	for _, itf := range ifaces {
		if itf.Flags&net.FlagUp != 0 {
			interfaces = append(interfaces, itf)
			graph.AddSeries(itf.Name,
				[]opts.LineData{},
				charts.WithMarkPointNameTypeItemOpts(opts.MarkPointNameTypeItem{Type: "max", Name: "Max"}),
				// charts.WithMarkLineNameTypeItemOpts(opts.MarkLineNameTypeItem{
				// 	Name: "Avg",
				// 	Type: "average",
				// }),
				charts.WithMarkPointStyleOpts(opts.MarkPointStyle{
					Label: &opts.Label{
						Show:      true,
						Formatter: opts.FuncOpts("function (params) { console.log(params) ;return formatBytes(params.value,'bps',2) }"),
					},
				}),
			)
		}
	}

	return &NetInterfacesViewer{graph: graph, direction: dir, interfaces: interfaces, lastvalues: GetNetStats(interfaces, dir), lastread: time.Now()}
}

func (vr *NetInterfacesViewer) SetStatsMgr(smgr *StatsMgr) {
	vr.smgr = smgr
}

func (vr *NetInterfacesViewer) Name() string {
	return VNetInterfaces + vr.direction
}

func (vr *NetInterfacesViewer) View() *charts.Line {
	return vr.graph
}

func (vr *NetInterfacesViewer) Serve(w http.ResponseWriter, _ *http.Request) {
	vr.smgr.Tick()

	now := time.Now()
	values := GetNetStats(vr.interfaces, vr.direction)
	for idx, value := range values {
		values[idx] = (value - vr.lastvalues[idx]) * 8 / now.Sub(vr.lastread).Seconds() // convert to bits then to bps
		vr.lastvalues[idx] = value
	}
	vr.lastread = now
	metrics := Metrics{
		Values: values,
		Time:   time.Now().Format(defaultCfg.TimeFormat),
	}
	bs, _ := json.Marshal(metrics)
	w.Write(bs)
}
