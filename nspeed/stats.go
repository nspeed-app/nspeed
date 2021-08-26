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
var MarchineInfoJSON, _ = json.MarshalIndent(MachineInfo, "", "  ")

func loadMachInfo() (retmachinfo machineInfo) {
	hostInfo, err := host.Info()
	if err == nil && hostInfo != nil {
		retmachinfo.OS = hostInfo.Platform + " " + hostInfo.PlatformVersion
		retmachinfo.Platform = hostInfo.OS
	}

	cpuInfo, err := cpu.Info()
	if err == nil && len(cpuInfo) > 0 {
		retmachinfo.CPUModel = cpuInfo[0].ModelName
	}

	retmachinfo.Hostname, _ = os.Hostname()
	retmachinfo.GoVersion = runtime.Version()
	retmachinfo.TotalMem = memory.TotalMemory()
	retmachinfo.Arch = runtime.GOARCH
	retmachinfo.NumCPU = runtime.NumCPU()
	retmachinfo.Time = time.Now()
	return retmachinfo
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
