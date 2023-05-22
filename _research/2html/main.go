package main

import (
	"fmt"
	"log"

	"nspeed.app/webview/statics/chartjs"
)

func main() {
	s, err := chartjs.GenDataset("a", "b", "c")
	if err != nil {
		log.Panic(err)
	}
	fmt.Println(s)
	s, err = chartjs.GenChart("a", "b", "c", "")
	if err != nil {
		log.Panic(err)
	}
	fmt.Println(s)
}
