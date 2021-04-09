package main

import (
	"fmt"
    "log"
    "github.com/StackExchange/wmi"
)

type Win32_PerfRawData_PerfOS_Processor struct {
    PercentProcessorTime uint64
    TimeStamp_Sys100NS   uint64
}

func main() {

    var rawOS []Win32_PerfRawData_PerfOS_Processor
    err := wmi.Query("SELECT * FROM Win32_PerfRawData_PerfOS_Processor where name='_Total'", &rawOS)
    if err != nil {
        log.Fatal(err)
    }

    // here comes stuff to work with rawOS... 
	fmt.Println(rawOS)
}
