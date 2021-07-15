package main

import (
	"context"
	"fmt"
	"os"

	"github.com/rs/zerolog"
	nspeed "nspeed.app/nspeed"
)

func main() {
	j := nspeed.JobCiphers{Target: os.Args[1]}
	j.Init(&nspeed.Metrics{
		Index:  0,
		Job:    &j,
		Logger: zerolog.Logger{},
	})
	fmt.Println(j.GoString())
	j.Run(context.Background())
}
