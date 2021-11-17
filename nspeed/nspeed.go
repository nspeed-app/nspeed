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
}

var GlobalOptions Options

// run all the jobs, this is a synchronous function
func RunJobs(jobs []Job, opt Options, ctx context.Context, progress func([]*Metrics)) ([]*Metrics, error) {

	jobs = ExpandJobs(jobs)

	njobs := len(jobs)
	results := make(chan *Metrics, njobs)
	wcWorkers := make([]*Metrics, njobs)

	fmt.Println("Jobs:")
	for idx, job := range jobs {
		wm := Metrics{
			Done:       false,
			Expired:    false,
			Index:      idx,
			Target:     job.GetTarget(),
			ResultChan: results,
			Logger:     log.With().Int("job", idx).Logger(),
			Job:        job}
		wcWorkers[idx] = &wm
		fmt.Printf("  %d %#v\n", idx, job)
	}

	// Cancel will allows to shutdown everything by calling cancel()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// add timeout (warning: possible shadowing errors with ctx & cancel)
	if opt.Timeout > 0 {
		newctx, cancel := context.WithTimeout(ctx, time.Duration(opt.Timeout)*time.Second)
		defer cancel()
		ctx = newctx
	}

	// create & init workers for jobs (synchronously one after the others)
	for _, wm := range wcWorkers {
		log.Debug().Msgf("creating worker %d %s %t", wm.Index, wm.Target, wm.Done)
		err := wm.Job.Init(wm)
		if err != nil {
			log.Error().Msgf("error %s for %s", err, wm.Target)
			return nil, err
		}
	}

	// run workers by launching a go routine for each one
	// todo:investigate using errgroup.WithContext (update: looking at errgroup.WithContext source code, it doesn't do much else)
	var wg sync.WaitGroup

	for _, wm := range wcWorkers {
		log.Debug().Msgf("spawning workload %d %s %t", wm.Index, wm.Target, wm.Done)
		wg.Add(1)
		//todo: use pprof.Do and labels ?
		go func(w *Metrics) {
			defer wg.Done()
			err := w.Job.Run(ctx)
			if err != nil {
				log.Error().Msgf("error '%s' for '%s'", err, w.Target)
				cancel()
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
	//fmt.Printf("zerolog.GlobalLevel = %d\n", zerolog.GlobalLevel())
	if zerolog.GlobalLevel() <= zerolog.InfoLevel {
		progressTicker := time.NewTicker(time.Duration(opt.ProgressInterval) * time.Second)
		fmt.Printf("setting progress ticker of %d seconds\n", opt.ProgressInterval)
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
			progress(wcWorkers)
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
