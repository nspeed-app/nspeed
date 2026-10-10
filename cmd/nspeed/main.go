package main

import (
	nspeed "nspeed.app/nspeed/cmd"
)

func main() {
	// f, err := os.Create("test/trace.out")
	// if err != nil {
	// 	panic("error creating trace file!" + err.Error())
	// }
	// trace.Start(f)
	// defer trace.Stop()
	nspeed.Main()
}
