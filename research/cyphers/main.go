package main

import (
	"context"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	nspeed "nspeed.app/nspeed"
)

func main() {
	j := nspeed.JobCiphers{Target: os.Args[1]}

	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	r := make(chan *nspeed.Metrics, 1)

	j.Init(&nspeed.Metrics{
		Index:      0,
		Job:        &j,
		Logger:     log.With().Str("source", "cmd line main").Logger(),
		ResultChan: r,
	})
	log.Warn().Msgf("running : %s", j.GetTarget())
	err := j.Run(context.Background())
	if err != nil {
		log.Fatal().Err(err).Send()
	}
	log.Warn().Msgf("done")
	wm := <-r
	wm.PrintFinal(os.Stdout)
}
