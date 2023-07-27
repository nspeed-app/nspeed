package nspeed

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Job interface {
	// per job implementation
	GetTarget() string
	GoString() string
	PrintString(bool, bool) string
	IsInfinite() bool
	Init() error
	Run(context.Context, *sync.WaitGroup) (*Metric, error)
	//common, implemented here
	Logger() *zerolog.Logger
	SetLogger(*zerolog.Logger)
	ID() string
	SetID(string)
	GroupID() string
	SetGroupID(string)
	Batch() int
	SetBatch(batch int)
}

// JobData should be embeded in every struct implementing Job interface
// it provides ID/GID storage
type JobData struct {
	Job
	id      string
	group   string
	batch   int
	jobList *JobList
	logger  *zerolog.Logger
}

func (j *JobData) ID() string            { return j.id }
func (j *JobData) SetID(id string)       { j.id = id }
func (j *JobData) GroupID() string       { return j.group }
func (j *JobData) SetGroupID(gid string) { j.group = gid }
func (j *JobData) Batch() int            { return j.batch }
func (j *JobData) SetBatch(batch int)    { j.batch = batch }

// Logger returns a dedicated Logger or the default one
func (j *JobData) Logger() *zerolog.Logger {
	if j.logger == nil {
		return &log.Logger
	}
	return j.logger
}
func (j *JobData) SetLogger(logger *zerolog.Logger) {
	if j.logger != nil && logger != nil {
		j.logger.Info().Msgf("overwriting a logger")
	}
	newlogger := logger.With().Str("jobid", j.ID()).Logger()
	j.logger = &newlogger
}

type Batch struct {
	Name      string
	PreDelay  time.Duration
	PostDelay time.Duration
}

// a list of jobs
type JobList struct {
	Jobs              map[string]Job
	Groups            map[string][]Job
	Batches           []*Batch
	currentBatchEmpty bool
}

func NewJoblist() *JobList {
	return &JobList{
		Jobs:              make(map[string]Job),
		Groups:            make(map[string][]Job),
		Batches:           make([]*Batch, 0, 1),
		currentBatchEmpty: false,
	}
}

// AddBatch adds a new batch
//
//	store batches by index
//	check for unique names
//	discard empty ones
func (jl *JobList) AddBatch(batch *Batch) error {
	if batch == nil {
		return fmt.Errorf("nil batch")
	}
	if batch.Name != "" {
		for _, b := range jl.Batches {
			if b.Name == batch.Name {
				return fmt.Errorf("batch named '%s' already exists", batch.Name)
			}
		}
	} else {
		batch.Name = "#" + strconv.Itoa(len(jl.Batches))
	}
	if !jl.currentBatchEmpty {
		jl.Batches = append(jl.Batches, batch)
	} else {
		jl.Batches[len(jl.Batches)-1] = batch
	}
	jl.currentBatchEmpty = true
	return nil
}

// AddJobs adds jobs as a group, preventing duplicate id/gid
// if a job name is empty, generate a unique one
// this is hard to read, rewrite properly
func (jl *JobList) AddJobs(jobs ...Job) error {
	if len(jl.Batches) == 0 {
		return fmt.Errorf("addjob but no batch yet")
	}
	batch := len(jl.Batches) - 1
	gid := "#" + strconv.Itoa(len(jl.Groups))
	n := len(jobs)

	var js []Job
	for i, j := range jobs {
		j.SetBatch(batch)
		jl.currentBatchEmpty = false
		if j == nil {
			return fmt.Errorf("func Addjob: nil argument")
		}
		if j.ID() == "" {
			j.SetID(gid)
		} else {
			gid = j.ID()
		}
		// group is ID for single job
		j.SetGroupID(gid)

		if n > 1 {
			j.SetID(j.ID() + "." + strconv.Itoa(i+1))
		}
		if _, ok := jl.Jobs[j.ID()]; ok {
			return fmt.Errorf("duplicate job id %s", j.ID())
		}
		jl.Jobs[j.ID()] = j
		js = append(js, j)
	}
	if _, ok := jl.Groups[gid]; ok {
		return fmt.Errorf("duplicate group id %s", gid)
	}
	jl.Groups[gid] = js
	return nil
}
