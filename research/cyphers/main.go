package main

import (
	"context"
	"os"

	nspeed "nspeed.app/nspeed"
)

func main() {
	j := nspeed.NSpeedCiphers{Target: os.Args[1]}
	j.Init(nil)
	j.Run(context.Background())
}
