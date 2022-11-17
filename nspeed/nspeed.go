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
		fmt.Printf("  %d-%#v\n", idx, job)
	}

	// Cancel will allows to shutdown everything by calling cancel()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// add timeout (warning: possible shadowing errors with ctx & cancel)
	if opt.Timeout > 0 {
		log.Debug().Dur("global_timeout", opt.Timeout).Msgf("setting global  timeout to %s", opt.Timeout)
		newctx, cancel := context.WithTimeout(ctx, opt.Timeout)
		defer cancel()
		ctx = newctx
	}

	// create & init workers for jobs (synchronously one after the others)
	// run infinite jobs
	for _, wm := range wcWorkers {
		log.Debug().Msgf("creating worker %d %s %t", wm.Index, wm.Target, wm.Done)
		err := wm.Job.Init(wm)
		if err != nil {
			log.Error().Msgf("error %s for %s", err, wm.Target)
			return nil, err
		}
	}

	// run workers by launching a go routine for each one
	// we use 2 waitgroup, one for short-lived jobs (get, put,etc), one for infinite (server, api)
	var wgsl sync.WaitGroup
	var wgi sync.WaitGroup

	IsInfinite := true // will be false if at least one job is not infinite
	for _, wm := range wcWorkers {
		log.Debug().Msgf("spawning workload %d %s %t", wm.Index, wm.Target, wm.Done)
		finalwg := &wgi
		if !wm.Job.IsInfinite() {
			finalwg = &wgsl
			IsInfinite = false
		}
		finalwg.Add(1)
		go func(w *Metrics, wg *sync.WaitGroup) {
			err := w.Job.Run(ctx, wg)
			if err != nil {
				log.Error().Msgf("error '%s' for '%s'", err, w.Target)
				cancel()
				return
			}
		}(wm, finalwg)
	}

	// wait for all infite jobs to signal they're ready
	log.Debug().Msg("waiting on infinite job to be ready")
	wgi.Wait()
	log.Debug().Msg("all infinite jobs are ready")

	// this waits for all (non infinite) jobs to end then sends a signal (purpose is to 'select' a waitgroup)
	cWaitgroupSLEnded := make(chan struct{})
	go func() {
		defer close(cWaitgroupSLEnded)
		wgsl.Wait()
		log.Debug().Msg("wait group SL ended")
	}()

	progressTickerChannel := make(<-chan time.Time) // forever
	//fmt.Printf("zerolog.GlobalLevel = %d\n", zerolog.GlobalLevel())
	if zerolog.GlobalLevel() <= zerolog.InfoLevel {
		progressTicker := time.NewTicker(time.Duration(opt.ProgressInterval) * time.Second)
		//fmt.Printf("setting progress ticker of %d seconds\n", opt.ProgressInterval)
		progressTickerChannel = progressTicker.C
	}

	if IsInfinite {
		fmt.Println("running - hit ctrl-C or send a kill signal to end")
	}
	rc := njobs
	fmt.Println("running...")
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
		case <-cWaitgroupSLEnded:
			log.Debug().Msgf("all sl workers ended (%d remaining active goroutines, rc = %d)", runtime.NumGoroutine(), rc)
			if !IsInfinite {
				break mainloop
			}
		}
	}
	log.Debug().Msgf("exited mainloop (%d remaining active goroutines, rc = %d)", runtime.NumGoroutine(), rc)
	fmt.Println("all done")

	// all done or canceled, return
	return wcWorkers, nil
}
