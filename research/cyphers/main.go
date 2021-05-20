package main

import (
	"context"
	"os"

	nspeed "nspeed.app/nspeed"
)

func main() {
	j := nspeed.NSpeedCyphers{Target: os.Args[1]}
	j.Start(context.Background(), nil)
}
