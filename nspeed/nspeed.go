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
	"nspeed.app/metric"
)

type Options struct {
	ProgressInterval time.Duration // logging/trace output rate
	Tick             time.Duration // metrics/series sampling rate
	CPU              time.Duration // cpu sampling rate
	Timeout          time.Duration
	Test             bool
	TLSSelf          bool
	IPVersion        int // 0, 4 or 6
}

var GlobalOptions Options

// RunJobs runs all the jobs, one batch at a time, this is a synchronous function
// update/progress can be obtained thru the optionnal message channel
func RunJobs(ctx context.Context, jl *JobList, opt Options, message chan any) (metric.Metrics, error) {

	jobs := jl.Jobs
	nJobs := len(jobs)
	nBatches := len(jl.Batches)
	if nBatches <= 0 {
		return nil, fmt.Errorf("no batch found")
	}

	jobIds := make([]string, 0, nJobs)
	for k := range jl.Jobs {
		jobIds = append(jobIds, k)
	}
	sort.Strings(jobIds)

	// set the logger for each job
	for _, idx := range jobIds {
		job := jl.Jobs[idx]
		logger := log.With().Str("job", idx).Logger()
		job.SetLogger(&logger)
		//wcWorkers = append(wcWorkers, &wm)
		msg := fmt.Sprintf("id=%s,gid=%s,batch=%s(%d)", idx, job.GroupID(), jl.Batches[job.Batch()].Name, job.Batch())
		msg2 := job.GoString()
		if opt.Test {
			logger.Warn().Msg(msg)
			logger.Warn().Msg(msg2)
		} else {
			logger.Info().Msg(msg)
			logger.Info().Msg(msg2)
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

	// this will collect result from jobs
	results := make(chan *metric.Metric, nJobs)
	// run workers by launching a go routine for each one
	// we use 2 waitgroup, one for short-lived jobs (get, put,etc), one for infinite jobs (server, api)
	var wgi sync.WaitGroup // infinite

	rc := nJobs
	wcWorkers := make(metric.Metrics, 0, nJobs)

	// batch loop

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

	batch := 0
	sljob := 0
	lljob := 0
	for {
		log.Info().Msgf("processing batch '%s' (%d)", jl.Batches[batch].Name, batch)
		llnj, err := runBatch(ctx, batch, jl, true, opt.Test, &wgi, results)
		if err != nil {
			cancel()
			return nil, err
		}
		lljob += llnj
		// wait for all infite jobs to signal they're ready
		log.Info().Msgf("waiting on %d infinite jobs to be ready", llnj)
		wgi.Wait()
		log.Info().Msg("all infinite jobs are ready")

		// spawn short lived jobs
		var wgsl sync.WaitGroup // short lived sync group
		// we add ourself to the waitgroup
		wgsl.Add(1)
		slnj, err := runBatch(ctx, batch, jl, false, opt.Test, &wgsl, results)
		if err != nil {
			cancel()
			return nil, err
		}
		sljob += slnj

		// isInfinite if last batch and no sljob seen
		isInfinite := sljob == 0 && batch == nBatches-1

		// this waits for all (non infinite) jobs to end then sends a signal
		// (purpose is to be able to 'select' the waitgroup)
		cWaitgroupSLEnded := make(chan struct{})
		go func() {
			defer close(cWaitgroupSLEnded)
			wgsl.Wait()
			log.Debug().Msg("wait group SL ended")
		}()

		if opt.Test {
			log.Warn().Msg("test mode - nothing done")
			return nil, nil
		}

		if isInfinite {
			log.Warn().Msg("running - hit ctrl-C or send a kill signal to end")
		} else {
			log.Warn().Msgf("running jobs of '%s' (%d)...", jl.Batches[batch].Name, batch)
		}

		if slnj <= 0 {
			log.Warn().Msgf("no SL jobs for batch %d", batch)
			wgsl.Done()
		}
	batchloop:
		for {
			log.Trace().Msgf("batchloop - select(%d remaining active goroutines, rc = %d)", runtime.NumGoroutine(), rc)
			select {
			case <-ctx.Done():
				log.Debug().Msgf("parent cancelled us")
				break batchloop
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
				// test for wm==nil ?
				wm.Logger().Debug().Msgf("got a result for job %s batch %d (err=%s)", wm.ID, batch, wm.Error)
				wm.Batch = batch
				wcWorkers = append(wcWorkers, wm)
				rc--
				slnj--
				if slnj == 0 {
					log.Info().Msgf("batch %s (%d) has more SL to do", jl.Batches[batch].Name, batch)
					wgsl.Done()
				}
				if wm.Done {
					wm.Logger().Info().Msgf("job %s ended - (on timeout: %t) - (%d remaining active goroutines, rc = %d)", wm.ID, wm.Expired, runtime.NumGoroutine(), rc)
				}
				if rc <= 0 {
					wm.Logger().Info().Msgf("no more jobs - exiting")
					break batchloop
				}
			case <-cWaitgroupSLEnded:
				log.Info().Msgf("all sl workers ended (%d remaining active goroutines, rc = %d)", runtime.NumGoroutine(), rc)
				if !isInfinite {
					log.Info().Msgf("no infinite, exiting")
					break batchloop
				}
				// we want this case to block the select forever
				cWaitgroupSLEnded = nil
			}
		}
		log.Info().Msgf("batch %d/%d ended", batch, nBatches)
		batch++
		if batch >= nBatches {
			cancel()
			break
		}
	}
	log.Info().Msgf("exited mainloop with %d results (%d remaining active goroutines, rc = %d)", len(wcWorkers), runtime.NumGoroutine(), rc)
	for len(wcWorkers) < nJobs {
		wm := <-results
		if wm != nil {
			wm.Logger().Info().Msgf("draining result")
			wcWorkers = append(wcWorkers, wm)
		}
	}

	// all done or canceled, return
	return wcWorkers, nil
}

func runBatch(ctx context.Context, batch int, jl *JobList, infinite bool, test bool, wg *sync.WaitGroup, results chan *metric.Metric) (int, error) {
	if jl == nil {
		return 0, fmt.Errorf("no job list")
	}
	jobs := jl.Jobs

	// PART1: INFINITE JOBS
	// create & init workers for jobs (synchronously one after the others)
	// run infinite jobs
	for i, j := range jobs {
		if j.Batch() != batch {
			continue
		}
		log.Debug().Msgf(" %s, %s", i, j.GoString())
		if j.IsInfinite() != infinite {
			continue
		}
		j.Logger().Debug().Msgf("creating worker %s %s", j.ID(), j.GetTarget())
		err := j.Init()
		if err != nil {
			if test && errors.Is(err, errNotRunning) {
				j.Logger().Warn().Msgf("(ok in test mode): error %s for %s", err, j.GetTarget())
			} else {
				j.Logger().Error().Msgf("error %s for %s", err, j.GetTarget())
				return 0, err
			}
		}
	}

	njobs := 0
	for _, j := range jobs {
		if j.Batch() != batch {
			continue
		}
		if j.IsInfinite() != infinite {
			continue
		}
		njobs++
		if !test {
			j.Logger().Debug().Msgf("spawning workload %s %s (infinite=%t)", j.ID(), j.GetTarget(), j.IsInfinite())
			wg.Add(1)
			go func(j Job, wg *sync.WaitGroup) {
				wm, err := j.Run(ctx, wg)
				if err != nil {
					log.Error().Msgf("error '%s' for '%s'", err, j.GetTarget())
				}
				j.Logger().Info().Msgf("sending result")
				results <- wm
			}(j, wg)
		} else {
			j.Logger().Debug().Msgf("test mode: skipping workload %s %s", j.ID(), j.GetTarget())
		}
	}
	return njobs, nil
}
