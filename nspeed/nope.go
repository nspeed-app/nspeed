package nspeed

import (
	"context"
)

// implements the "nope" command which doesn't nothing and wait for ever
type NopeJob struct {
}

func (j *NopeJob) GetTarget() string {
	return "Nope"
}

func (j *NopeJob) Expand(jobs []Job, group int) []Job {
	return append(jobs, j)
}

func (j *NopeJob) GoString() string {
	return "Nope"
}

func (j *NopeJob) PrintString(verbose bool) string {
	return "Nope"
}

func (j *NopeJob) IsInfinite() bool {
	return true
}

func (s *NopeJob) Init(wm *Metrics) error {
	return nil
}

// just wait for the context
func (s *NopeJob) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}
