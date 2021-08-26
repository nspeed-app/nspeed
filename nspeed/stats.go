package nspeed

import (
	"os"
	"runtime"
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

type machineStats struct {
	CPU            float64 `json:"cpu"`
	MemUsedPercent float64 `json:"mempercent"`
	GoMemory       int64   `json:"gomem"`
	GoMemorySys    int64   `json:"gomemsys"`
	GoRoutines     int     `json:"goroutines"`
}

var MachineInfo machineInfo = loadMachInfo()

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

var MachineStats machineStats

func (s *machineStats) LoadStats(diskDir string, interval time.Duration) {
	//count cpu cycles between last count
	if cpu, err := cpu.Percent(interval, false); err == nil {
		if len(cpu) > 0 {
			s.CPU = cpu[0]
		}
	}
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
}
