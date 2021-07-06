package nspeed

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
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

func Run(jobs []Job, opt Options, ctx context.Context, progress func([]*Metrics)) ([]*Metrics, error) {

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
			LogLevel:   consoleLevel, //todo
			Logger:     log.With().Int("job", idx).Logger(),
			Job:        job}
		wcWorkers[idx] = &wm
		fmt.Printf("  %d %#v\n", idx, job)
		// todo check valid url, prefetch, do dns if needed, add context ? etc
	}

	// shutdownCh triggers graceful shutdown on SIGINT or SIGTERM
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	// Cancel will allows to shutdown everything on
	ctx, Cancel := context.WithCancel(ctx)
	defer Cancel()

	// add timeout
	if opt.Timeout > 0 {
		newctx, cancel := context.WithTimeout(ctx, time.Duration(*optTimeout)*time.Second)
		defer cancel()
		ctx = newctx
	}

	// create & init workers for jobs (sync one after the others)
	for _, wm := range wcWorkers {
		log.Debug().Msgf("creating workload %d %s %t", wm.Index, wm.Target, wm.Done)
		err := wm.Job.Init(wm)
		if err != nil {
			log.Error().Msgf("error %s for %s", err, wm.Target)
			return nil, err
		}
	}

	// run workers by launching a go routine for each one
	//
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

	// main thread loop
	// - wait for (partial) result from workers
	// - wait for progress tick and print results
	// - wait for all workers to have ended or timeout'ed
	// - handle ctrl-c & errors

	cWaitgroupEnded := make(chan struct{})
	go func() {
		defer close(cWaitgroupEnded)
		wg.Wait()
		log.Debug().Msg("wait group ended")
	}()

	rc := njobs

	done := false
	if opt.Test {
		done = true
	}

	progressTickerChannel := make(<-chan time.Time) // forever
	progressCPUChannel := make(<-chan time.Time)    // forever
	if *optCPU {
		progressCPUTicker := time.NewTicker(time.Duration(*optTick) * time.Second) // todo: pass value
		progressCPUChannel = progressCPUTicker.C
	}
	if zerolog.GlobalLevel() <= zerolog.InfoLevel {
		progressTicker := time.NewTicker(time.Duration(*optTick) * time.Second)
		progressTickerChannel = progressTicker.C
	}

	for !done {
		select {
		case sig := <-shutdownCh:
			log.Warn().Msgf("global shutdown started by signal: %s", sig)
			Cancel()
		case <-progressCPUChannel:
			if *optCPU {
				cpuload := nspeed.GetCPULoad(*optColor)
				// we dont log.Info() this to avoid sending it to the file
				log.Log().Msgf("%s, active jobs: %d / active goroutines: %d", cpuload, rc, runtime.NumGoroutine())
			}
		case <-progressTickerChannel:
			// time critical: warning for overlap/backlogs if what's done here takes too long
			// this section assumes loglevel >= info
			for _, wm := range wcWorkers {
				if !wm.Done && (wm.LogLevel == zerolog.DebugLevel) {
					wm.LogProgress(&wm.Logger)
				}
			}
			PrintMemUsage(zerolog.GlobalLevel())
		case wm := <-results:
			rc--
			if wm.Done {
				log.Debug().Msgf("job %d ended - (on timeout: %t) - (%d remaining active goroutines)", wm.Index, wm.Expired, runtime.NumGoroutine())
			}
		case <-cWaitgroupEnded:
			log.Debug().Msgf("all workers done (%d remaining active goroutines)", runtime.NumGoroutine())
			done = true
		}
	}
	log.Debug().Msgf("main loop ended. active jobs: %d / active goroutines: %d", rc, runtime.NumGoroutine())
	log.Debug().Msg("sleeping one second")
	time.Sleep(1 * time.Second)
}
