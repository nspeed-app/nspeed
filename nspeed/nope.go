package nspeed

import (
	"context"
)

// implements the "nope" command which doesn't nothing and wait for ever
type Nope struct {
}

func (j *Nope) GetTarget() string {
	return "Nope"
}

func (j *Nope) Expand(jobs []Job, group int) []Job {
	return append(jobs, j)
}

func (j *Nope) GoString() string {
	return "Nope"
}

func (j *Nope) PrintString(verbose bool) string {
	return "Nope"
}

func (j *Nope) IsInfinite() bool {
	return true
}

func (s *Nope) Init(wm *Metrics) error {
	return nil
}

// just wait for the context
func (s *Nope) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}
