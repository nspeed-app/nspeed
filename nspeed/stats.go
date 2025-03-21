package nspeed

import (
	"encoding/json"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/pbnjay/memory"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

// greatly inspired by https://github.com/varbhat/exatorrent

type machineInfo struct {
	Arch     string    `json:"arch"`
	NumCPU   int       `json:"numcpu"`
	CPUModel string    `json:"cpumodel"`
	Hostname string    `json:"hostname"`
	Platform string    `json:"platform"`
	OS       string    `json:"os"`
	TotalMem uint64    `json:"totalmem"`
	Time     time.Time `json:"time"`
}

type memStats struct {
	MemUsedPercent float64 `json:"mempercent"`
	GoMemory       int64   `json:"gomem"`
	GoMemorySys    int64   `json:"gomemsys"`
	GoRoutines     int     `json:"goroutines"`
	Time           time.Time
}

var MachineInfo = loadMachInfo()
var MachineInfoMap map[string]any
var MachineInfoJSON, _ = StructToJSON(MachineInfo, &MachineInfoMap)

// StructToJSON converts a Go struct into a map[string]any where each entry is the json name of the struct members and their value
// for instance:
//
//	type info struct {
//	  GoVal int `json:"jsonval"`
//	}
//	i := info{GoVal:10}
//	var j map[string]any
//	var k,err = StructToJSON(i,&j)
//
// k will contains a single entry with key "jsonval" and value 10
//
// see https://go.dev/play/p/mdSOnWD-axj
func StructToJSON(v any, m *map[string]any) ([]byte, error) {
	z, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(z, m)
	return z, err
}

func loadMachInfo() (r machineInfo) {
	hostInfo, err := host.Info()
	if err == nil && hostInfo != nil {
		r.OS = hostInfo.Platform + " " + hostInfo.PlatformVersion
		r.Platform = hostInfo.OS
	}

	cpuInfo, err := cpu.Info()
	// should maybe build a set of all ModelName ?
	if err == nil && len(cpuInfo) > 0 {
		r.CPUModel = cpuInfo[0].ModelName
	}

	r.Hostname, _ = os.Hostname()
	//r.GoVersion = runtime.Version()
	r.TotalMem = memory.TotalMemory()
	r.Arch = runtime.GOARCH
	r.NumCPU = runtime.NumCPU()
	r.Time = time.Now()
	return r
}

var MemStats memStats
var memStatsMu sync.Mutex // could put this in the struct

func (s *memStats) LoadStats() {
	memStatsMu.Lock()
	defer memStatsMu.Unlock()
	//count memory usage
	if stat, err := mem.VirtualMemory(); err == nil {
		s.MemUsedPercent = stat.UsedPercent
	}
	//count total bytes allocated by the go runtime
	memStats := runtime.MemStats{}
	runtime.ReadMemStats(&memStats)
	s.GoMemory = int64(memStats.Alloc)
	s.GoMemorySys = int64(memStats.Sys)
	//count current number of goroutines
	s.GoRoutines = runtime.NumGoroutine()
	s.Time = time.Now()
}
