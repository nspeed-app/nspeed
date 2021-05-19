package main

import (
	"os"

	nspeed "nspeed.app/nspeed"
)

func main() {
	nspeed.PrintCyphers(os.Args[1])
}
