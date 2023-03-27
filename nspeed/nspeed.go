package nspeed

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type Options struct {
	ProgressInterval time.Duration // logging/trace output rate
	Tick             time.Duration // metrics/series sampling rate
	CPU              time.Duration // cpu sampling rate
	Timeout          time.Duration
	Test             bool
	TLSSelf          bool
}

var GlobalOptions Options

// RunJobs runs all the jobs, this is a synchronous function
func RunJobs(ctx context.Context, jl *JobList, opt Options, message chan any) (Metrics, error) {

	jobs := jl.Jobs
	njobs := len(jobs)

	jobIds := make([]string, 0, njobs)
	for k := range jl.Jobs {
		jobIds = append(jobIds, k)
	}
	sort.Strings(jobIds)

	// this will collect result from jobs
	results := make(chan *Metric, njobs)

	for _, id := range jobIds {
		job := jl.Jobs[id]
		wm := Metric{
			MetricsData: MetricsData{
				Done:    false,
				Expired: false,
				Target:  job.GetTarget(),
			},
			Logger: log.With().Str("job", id).Logger(),
			Tick:   opt.Tick,
		}
		job.SetLogger(&wm.Logger)
		//wcWorkers = append(wcWorkers, &wm)
		msg := fmt.Sprintf("id=%s,gid=%s,%#v", id, job.GroupID(), job)
		if opt.Test {
			wm.Logger.Warn().Msg(msg)
		} else {
			wm.Logger.Info().Msg(msg)
		}
	}

	// cancel will allows to shutdown everything by calling cancel()
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
	log.Debug().Msgf("total jobs = %d", len(jobs))
	for i, j := range jobs {
		log.Debug().Msgf(" %s, %s", i, j.GoString())
		if !j.IsInfinite() {
			continue
		}
		j.Logger().Debug().Msgf("creating worker %s %s", j.ID(), j.GetTarget())
		err := j.Init()
		if err != nil {
			log.Error().Msgf("error %s for %s", err, j.GetTarget())
			return nil, err
		}
	}

	// run workers by launching a go routine for each one
	// we use 2 waitgroup, one for short-lived jobs (get, put,etc), one for infinite jobs (server, api)
	var wgi sync.WaitGroup // infinite

	for _, j := range jobs {
		if !j.IsInfinite() {
			continue
		}
		if !opt.Test {
			j.Logger().Debug().Msgf("spawning infinite workload %s %s", j.ID(), j.GetTarget())
			wgi.Add(1)
			go func(j Job, wg *sync.WaitGroup) {
				err := j.Run(ctx, results, wg)
				if err != nil {
					log.Error().Msgf("error '%s' for '%s'", err, j.GetTarget())
					cancel()
					return
				}
			}(j, &wgi)
		} else {
			j.Logger().Debug().Msgf("test mode: skipping workload %s %s", j.ID(), j.GetTarget())
		}
	}
	// wait for all infite jobs to signal they're ready
	log.Info().Msg("waiting on infinite jobs to be ready")
	wgi.Wait()
	log.Info().Msg("all infinite jobs are ready")

	// PART2: SHORT LIVED JOBS
	// spawn short lived jobs
	for _, j := range jobs {
		if j.IsInfinite() {
			continue
		}
		j.Logger().Debug().Msgf("creating worker %s %s", j.ID(), j.GetTarget())
		err := j.Init()
		if err != nil {
			if opt.Test && errors.Is(err, errNotRunning) {
				j.Logger().Warn().Msgf("(ok in test mode): error %s for %s", err, j.GetTarget())
			} else {
				j.Logger().Error().Msgf("error %s for %s", err, j.GetTarget())
			}
		}
	}

	var wgsl sync.WaitGroup // short lived sync group
	isInfinite := true      // will be false if at least one job is not infinite
	for _, j := range jobs {
		if j.IsInfinite() {
			continue
		}
		isInfinite = false //we got at least one finite job
		if !opt.Test {
			j.Logger().Debug().Msgf("spawning workload %s %s", j.ID(), j.GetTarget())
			wgsl.Add(1)
			go func(j Job, wg *sync.WaitGroup) {
				err := j.Run(ctx, results, wg)
				if err != nil {
					j.Logger().Error().Msgf("error '%s' for '%s'", err, j.GetTarget())
					cancel()
					return
				}
			}(j, &wgsl)
		} else {
			j.Logger().Debug().Msgf("test mode: skipping workload %s %s", j.GetTarget(), j.GetTarget())
		}
	}

	// this waits for all (non infinite) jobs to end then sends a signal (purpose is to 'select' a waitgroup)
	cWaitgroupSLEnded := make(chan struct{})
	go func() {
		defer close(cWaitgroupSLEnded)
		wgsl.Wait()
		log.Debug().Msg("wait group SL ended")
	}()

	// Ticker for progress
	progressTickerChannel := make(<-chan time.Time) // forever
	if message != nil && opt.ProgressInterval > 0 {
		t := time.NewTicker(opt.ProgressInterval)
		log.Debug().Msgf("setting progress ticker to %s", opt.ProgressInterval)
		progressTickerChannel = t.C
	}

	// Ticker for time series
	tickTickerChannel := make(<-chan time.Time) // forever
	if message != nil && opt.Tick > 0 {
		t := time.NewTicker(opt.Tick)
		log.Debug().Msgf("setting series ticker to %s", opt.Tick)
		tickTickerChannel = t.C
	}

	// Ticker for CPU series
	tickCPUChannel := make(<-chan time.Time) // forever
	if message != nil && opt.CPU > 0 {
		t := time.NewTicker(opt.CPU) // todo: pass value
		log.Debug().Msgf("setting CPU ticker to %s", opt.CPU)
		tickCPUChannel = t.C
	}

	if opt.Test {
		fmt.Println("test mode - nothing done")
		return nil, nil
	}

	if isInfinite {
		fmt.Println("running - hit ctrl-C or send a kill signal to end")
	} else {
		fmt.Println("running...")
	}
	rc := njobs
	wcWorkers := make(Metrics, 0, njobs)
mainloop:
	for {
		log.Trace().Msgf("mainloop - select(%d remaining active goroutines, rc = %d)", runtime.NumGoroutine(), rc)
		select {
		case <-ctx.Done():
			log.Debug().Msgf("parent cancelled us")
			break mainloop
		case <-tickTickerChannel:
			// we call Update on each wcWorkers
			for _, wc := range wcWorkers {
				wc.UpdateData()
			}
		case <-tickCPUChannel:
			pcts, err := GetCPULoad(0, true)
			if err != nil {
				log.Log().Err(err).Msgf("failed to get cpu load")
			}
			message <- CPUMetric{pcts, rc}
		case <-progressTickerChannel:
			message <- wcWorkers
		case wm := <-results:
			wcWorkers = append(wcWorkers, wm)
			rc--
			if wm.Done {
				wm.Logger.Info().Msgf("job %s ended - (on timeout: %t) - (%d remaining active goroutines, rc = %d)", wm.ID, wm.Expired, runtime.NumGoroutine(), rc)
			}
			if rc <= 0 {
				wm.Logger.Info().Msgf("no more jobs - exiting")
				break mainloop
			}
		case <-cWaitgroupSLEnded:
			log.Debug().Msgf("all sl workers ended (%d remaining active goroutines, rc = %d)", runtime.NumGoroutine(), rc)
			if !isInfinite {
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
