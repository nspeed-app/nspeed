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

// RunJobs runs all the jobs, one batch at a time, this is a synchronous function
func RunJobs(ctx context.Context, jl *JobList, opt Options, message chan any) (Metrics, error) {

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
		msg := fmt.Sprintf("id=%s,gid=%s,batch=%s(%d),%#v", idx, job.GroupID(), jl.Batches[job.Batch()].Name, job.Batch(), job)
		if opt.Test {
			logger.Warn().Msg(msg)
		} else {
			logger.Info().Msg(msg)
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
	results := make(chan *Metric, nJobs)
	// run workers by launching a go routine for each one
	// we use 2 waitgroup, one for short-lived jobs (get, put,etc), one for infinite jobs (server, api)
	var wgi sync.WaitGroup // infinite

	rc := nJobs
	wcWorkers := make(Metrics, 0, nJobs)

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
	for {

		nj, err := runBatch(ctx, batch, jl, true, opt.Test, &wgi, results)
		if err != nil {
			cancel()
			return nil, err
		}
		// wait for all infite jobs to signal they're ready
		log.Info().Msgf("waiting on %d infinite  jobs to be ready", nj)
		wgi.Wait()
		log.Info().Msg("all infinite jobs are ready")

		// spawn short lived jobs
		var wgsl sync.WaitGroup // short lived sync group
		nj, err = runBatch(ctx, batch, jl, false, opt.Test, &wgsl, results)
		if err != nil {
			cancel()
			return nil, err
		}
		sljob += nj
		isInfinite := sljob == 0 && batch == nBatches-1
		// this waits for all (non infinite) jobs to end then sends a signal (purpose is to 'select' a waitgroup)
		cWaitgroupSLEnded := make(chan struct{})
		go func() {
			defer close(cWaitgroupSLEnded)
			wgsl.Wait()
			log.Debug().Msg("wait group SL ended")
		}()

		if opt.Test {
			fmt.Println("test mode - nothing done")
			return nil, nil
		}

		if isInfinite {
			fmt.Println("running - hit ctrl-C or send a kill signal to end")
		} else {
			fmt.Printf("running batch %s...\n", jl.Batches[batch].Name)
		}
	batchloop:
		for {
			log.Trace().Msgf("mainloop - select(%d remaining active goroutines, rc = %d)", runtime.NumGoroutine(), rc)
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
				wm.Logger.Debug().Msgf("got a result for job %s batch %d", wm.ID, batch)
				wm.Batch = batch
				wcWorkers = append(wcWorkers, wm)
				rc--
				if wm.Done {
					wm.Logger.Info().Msgf("job %s ended - (on timeout: %t) - (%d remaining active goroutines, rc = %d)", wm.ID, wm.Expired, runtime.NumGoroutine(), rc)
				}
				if rc <= 0 {
					wm.Logger.Info().Msgf("no more jobs - exiting")
					break batchloop
				}
			case <-cWaitgroupSLEnded:
				log.Debug().Msgf("all sl workers ended (%d remaining active goroutines, rc = %d)", runtime.NumGoroutine(), rc)
				if !isInfinite {
					break batchloop
				}
				// we want this case to block the select forever
				cWaitgroupSLEnded = nil
			}
		}
		log.Debug().Msgf("batch %d/%d ended", batch, nBatches)
		batch++
		if batch >= nBatches {
			break
		}
	}
	log.Info().Msgf("exited mainloop with %d results (%d remaining active goroutines, rc = %d)", len(wcWorkers), runtime.NumGoroutine(), rc)

	// all done or canceled, return
	return wcWorkers, nil
}

func runBatch(ctx context.Context, batch int, jl *JobList, infinite bool, test bool, wg *sync.WaitGroup, results chan *Metric) (int, error) {
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
			j.Logger().Debug().Msgf("spawning infinite workload %s %s", j.ID(), j.GetTarget())
			wg.Add(1)
			go func(j Job, wg *sync.WaitGroup) {
				err := j.Run(ctx, results, wg)
				if err != nil {
					log.Error().Msgf("error '%s' for '%s'", err, j.GetTarget())
					return
				}
			}(j, wg)
		} else {
			j.Logger().Debug().Msgf("test mode: skipping workload %s %s", j.ID(), j.GetTarget())
		}
	}
	return njobs, nil
}
