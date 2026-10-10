package main

import (
	"fmt"
	"log"
	"sort"

	"github.com/shirou/gopsutil/process"
)

type proc struct {
	Id   int32
	Name string
	Pct  float64
}

func main() {
	lprocs, err := process.Processes()
	if err != nil {
		log.Fatal(err)
	}

	// get top 10 processes by cpu%
	processes := make([]proc, 0, len(lprocs))
	for _, p := range lprocs {
		//name, _ := p.Name()
		pct, err := p.CPUPercent()
		if err != nil {
			continue
		}
		proc := proc{
			Id:   p.Pid,
			Name: "todo",
			Pct:  pct,
		}
		processes = append(processes, proc)
	}
	fmt.Printf("got %d process\n", len(processes))

	sort.Slice(processes, func(i, j int) bool {
		return processes[i].Pct > processes[j].Pct
	})
	for i := range 10 {
		fmt.Println(i, processes[i])
	}
}
