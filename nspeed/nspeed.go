package nspeed

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Options struct {
	ProgressInterval int
	Timeout          time.Duration
	Test             bool
	TLSSelf          bool
	ConsoleLevel     zerolog.Level
}

// run all the jobs, this is a synchronous function
func RunJobs(jobs []Job, opt Options, ctx context.Context, progress func([]*Metrics)) ([]*Metrics, error) {

	jobs = ExpandJobs(jobs)

	njobs := len(jobs)
	results := make(chan *Metrics, njobs)
	wcWorkers := make([]*Metrics, njobs)

	//IsInfinite := false
	fmt.Println("Jobs:")
	for idx, job := range jobs {
		wm := Metrics{
			Done:       false,
			Expired:    false,
			Index:      idx,
			Target:     job.GetTarget(),
			ResultChan: results,
			LogLevel:   opt.ConsoleLevel, //todo
			Logger:     log.With().Int("job", idx).Logger(),
			Job:        job}
		wcWorkers[idx] = &wm
		fmt.Printf("  %d %#v\n", idx, job)
		// todo check valid url, prefetch, do dns if needed, add context ? etc
	}

	// Cancel will allows to shutdown everything on
	ctx, Cancel := context.WithCancel(ctx)
	defer Cancel()

	// add timeout
	if opt.Timeout > 0 {
		newctx, cancel := context.WithTimeout(ctx, time.Duration(opt.Timeout)*time.Second)
		defer cancel()
		ctx = newctx
	}

	// create & init workers for jobs (synchronously one after the others)
	for _, wm := range wcWorkers {
		log.Debug().Msgf("creating workload %d %s %t", wm.Index, wm.Target, wm.Done)
		err := wm.Job.Init(wm)
		if err != nil {
			log.Error().Msgf("error %s for %s", err, wm.Target)
			return nil, err
		}
	}

	// run workers by launching a go routine for each one
	var wg sync.WaitGroup
	wg.Add(njobs)
	for _, wm := range wcWorkers {
		log.Debug().Msgf("spawning workload %d %s %t", wm.Index, wm.Target, wm.Done)
		go func(w *Metrics) {
			defer wg.Done()
			err := w.Job.Run(ctx)
			if err != nil {
				log.Error().Msgf("error '%s' for '%s'", err, w.Target)
				Cancel()
				return
			}
		}(wm)
	}

	// this waits for all jobs to end then sends a signal (purpose is to 'select' a waitgroup)
	cWaitgroupEnded := make(chan struct{})
	go func() {
		defer close(cWaitgroupEnded)
		wg.Wait()
		log.Debug().Msg("wait group ended")
	}()

	progressTickerChannel := make(<-chan time.Time) // forever
	if zerolog.GlobalLevel() <= zerolog.InfoLevel {
		progressTicker := time.NewTicker(time.Duration(opt.ProgressInterval) * time.Second)
		progressTickerChannel = progressTicker.C
	}

	rc := njobs
mainloop:
	for {
		select {
		case <-ctx.Done():
			log.Debug().Msgf("parent cancelled us")
			break mainloop
		case <-progressTickerChannel:
			// time critical: warning for overlap/backlogs if what's done here takes too long
			// this section assumes loglevel >= info
			for _, wm := range wcWorkers {
				if !wm.Done && (wm.LogLevel == zerolog.DebugLevel) {
					wm.LogProgress(&wm.Logger)
				}
			}
			//PrintMemUsage(zerolog.GlobalLevel())
		case wm := <-results:
			rc--
			if wm.Done {
				log.Debug().Msgf("job %d ended - (on timeout: %t) - (%d remaining active goroutines, rc = %d)", wm.Index, wm.Expired, runtime.NumGoroutine(), rc)
			}
		case <-cWaitgroupEnded:
			log.Debug().Msgf("all workers ended (%d remaining active goroutines, rc = %d)", runtime.NumGoroutine(), rc)
			break mainloop
		}
	}

	// all done or canceled, return
	return wcWorkers, nil
}
