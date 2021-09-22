package webview

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
)

const (
	//
	VNetInterfaces = "Network Interfaces"
)

// NetInterfacesViewer collects network interfaces counters
type NetInterfacesViewer struct {
	smgr       *StatsMgr
	graph      *charts.Line
	direction  string
	interfaces []string
	lastvalues []float64
	lastread   time.Time
}

// NewNetInterfacesViewer returns the NetInterfacesViewer instance
// Series: net interfaces
func NewNetInterfacesViewer(dir string) Viewer {
	graph := newBasicView(VNetInterfaces + dir)
	graph.SetGlobalOptions(
		charts.WithYAxisOpts(opts.YAxis{
			Name:      dir,
			AxisLabel: &opts.AxisLabel{Formatter: "{value}"},
		}),
		charts.WithLegendOpts(opts.Legend{Show: true}),
	)
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Fatalf("error calling net.Interfaces(): %s", err)
	}
	var interfaces []string
	for _, itf := range ifaces {
		if itf.Flags&net.FlagUp != 0 {
			interfaces = append(interfaces, itf.Name)
			graph.AddSeries(itf.Name, []opts.LineData{})
		}
	}
	return &NetInterfacesViewer{graph: graph, direction: dir, interfaces: interfaces, lastvalues: getNetStats(interfaces, dir), lastread: time.Now()}
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
	values := getNetStats(vr.interfaces, vr.direction)
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

// return rx/tx counters for interfaces - Linux version
func getNetStats(interfaces []string, dir string) []float64 {
	var values []float64
	for _, itf := range interfaces {
		var value int
		file, err := os.Open(fmt.Sprintf("/sys/class/net/%s/statistics/%s_bytes", itf, dir))
		if err != nil {
			value = -1
		} else {
			fmt.Fscanf(file, "%d", &value)
		}
		values = append(values, float64(value))
	}
	return values
}
