package nspeed

import (
	"context"
	"errors"
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
func RunJobs(jl *JobList, opt Options, ctx context.Context, progress func([]*Metrics)) ([]*Metrics, error) {

	jobs := jl.Jobs

	njobs := len(jobs)
	results := make(chan *Metrics, njobs)
	wcWorkers := make([]*Metrics, njobs)

	for idx, job := range jobs {
		wm := Metrics{
			MetricsData: MetricsData{
				Done:    false,
				Expired: false,
				Index:   idx,
				Target:  job.GetTarget(),
				Job:     job,
			},
			ResultChan: results,
			Logger:     log.With().Int("job", idx).Logger(),
		}
		wcWorkers[idx] = &wm
		msg := fmt.Sprintf("job %d (id=%s,gid=%s) = %#v", idx, job.ID(), job.GroupID(), job)
		if opt.Test {
			log.Warn().Msg(msg)
		} else {
			log.Info().Msg(msg)
		}
		//fmt.Printf("  %d-%#v\n", idx, job)
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

	// PART1: INFINITE JOBS
	// create & init workers for jobs (synchronously one after the others)
	// run infinite jobs
	for _, wm := range wcWorkers {
		if !wm.Job.IsInfinite() {
			continue
		}
		log.Debug().Msgf("creating worker %d %s %t", wm.Index, wm.Target, wm.Done)
		err := wm.Job.Init(wm)
		if err != nil {
			log.Error().Msgf("error %s for %s", err, wm.Target)
			return nil, err
		}
	}

	// run workers by launching a go routine for each one
	// we use 2 waitgroup, one for short-lived jobs (get, put,etc), one for infinite jobs (server, api)
	var wgi sync.WaitGroup // infinite

	for _, wm := range wcWorkers {
		if !wm.Job.IsInfinite() {
			continue
		}
		if !opt.Test {
			log.Debug().Msgf("spawning infinite workload %d %s %t", wm.Index, wm.Target, wm.Done)
			wgi.Add(1)
			go func(w *Metrics, wg *sync.WaitGroup) {
				err := w.Job.Run(ctx, wg)
				if err != nil {
					log.Error().Msgf("error '%s' for '%s'", err, w.Target)
					cancel()
					return
				}
			}(wm, &wgi)
		} else {
			log.Debug().Msgf("test mode: skipping workload %d %s %t", wm.Index, wm.Target, wm.Done)
		}
	}
	// wait for all infite jobs to signal they're ready
	log.Info().Msg("waiting on infinite jobs to be ready")
	wgi.Wait()
	log.Info().Msg("all infinite jobs are ready")

	// PART2: SHORT LIVED JOBS
	// spawn short lived jobs
	for _, wm := range wcWorkers {
		if wm.Job.IsInfinite() {
			continue
		}
		log.Debug().Msgf("creating worker %d %s %t", wm.Index, wm.Target, wm.Done)
		err := wm.Job.Init(wm)
		if err != nil {
			if opt.Test && errors.Is(err, errNotRunning) {
				log.Warn().Msgf("(ok in test mode): error %s for %s", err, wm.Target)
			} else {
				log.Error().Msgf("error %s for %s", err, wm.Target)
			}
		}
	}

	var wgsl sync.WaitGroup // short lived sync group
	IsInfinite := true      // will be false if at least one job is not infinite
	for _, wm := range wcWorkers {
		if wm.Job.IsInfinite() {
			continue
		}
		IsInfinite = false //we got at least one finite job
		if !opt.Test {
			log.Debug().Msgf("spawning workload %d %s %t", wm.Index, wm.Target, wm.Done)
			wgsl.Add(1)
			go func(w *Metrics, wg *sync.WaitGroup) {
				err := w.Job.Run(ctx, wg)
				if err != nil {
					log.Error().Msgf("error '%s' for '%s'", err, w.Target)
					cancel()
					return
				}
			}(wm, &wgsl)
		} else {
			log.Debug().Msgf("test mode: skipping workload %d %s %t", wm.Index, wm.Target, wm.Done)
		}
	}

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

	if opt.Test {
		fmt.Println("test mode - nothing done")
		return nil, nil
	}

	if IsInfinite {
		fmt.Println("running - hit ctrl-C or send a kill signal to end")
	} else {
		fmt.Println("running...")
	}
	rc := njobs
mainloop:
	for {
		log.Debug().Msgf("mainloop - select(%d remaining active goroutines, rc = %d)", runtime.NumGoroutine(), rc)
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
			// we want this case to block the select forever
			cWaitgroupSLEnded = nil
		}
	}
	log.Debug().Msgf("exited mainloop (%d remaining active goroutines, rc = %d)", runtime.NumGoroutine(), rc)
	fmt.Println("all done")

	// all done or canceled, return
	return wcWorkers, nil
}
