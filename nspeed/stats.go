package nspeed

import (
	"encoding/json"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/pbnjay/memory"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/mem"
)

// greatly inspired by https://github.com/varbhat/exatorrent

type machineInfo struct {
	Arch      string    `json:"arch"`
	NumCPU    int       `json:"numcpu"`
	CPUModel  string    `json:"cpumodel"`
	Hostname  string    `json:"hostname"`
	Platform  string    `json:"platform"`
	OS        string    `json:"os"`
	TotalMem  uint64    `json:"totalmem"`
	GoVersion string    `json:"goversion"`
	Time      time.Time `json:"time"`
}

type memStats struct {
	MemUsedPercent float64 `json:"mempercent"`
	GoMemory       int64   `json:"gomem"`
	GoMemorySys    int64   `json:"gomemsys"`
	GoRoutines     int     `json:"goroutines"`
	Time           time.Time
}

var MachineInfo machineInfo = loadMachInfo()
var MachineInfoMap map[string]interface{}
var MachineInfoJSON, _ = StructToJson(MachineInfo, &MachineInfoMap)

func StructToJson(v interface{}, m *map[string]interface{}) ([]byte, error) {
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
	if err == nil && len(cpuInfo) > 0 {
		r.CPUModel = cpuInfo[0].ModelName
	}

	r.Hostname, _ = os.Hostname()
	r.GoVersion = runtime.Version()
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
