package nspeed

import (
	"context"
	"errors"
	"fmt"
)

/*
- ABI
	AddJobs(...cb_progress )
	Run( cb_progress )
	Stop() / Cancel
	GetResults()


- Web API

	AddJobs(...cb_progress )
	Run( ws-progress )
	Stop() / Cancel
	GetResults()
*/

type APIHost struct {
	Parameters *NSpeedHost
	Server     *NSpeedHTTPServer // underlying HTTP server
}

func (j *APIHost) GetTarget() string {
	return fmt.Sprintf("server %s:%d (IP version: %d)", j.Parameters.Host, j.Parameters.Port, j.Parameters.ForceIPVersion)
}

func (j *APIHost) Expand(jobs []Job) []Job {
	return append(jobs, j)
}

func (j *APIHost) GoString() string {
	return fmt.Sprintf("API {Address: %q, port: %d, IPversion: %d, UseTLS: %t}",
		j.Parameters.Host,
		j.Parameters.Port,
		j.Parameters.ForceIPVersion,
		j.Parameters.ServeTLS,
	)
}

func (j *APIHost) PrintString() string {
	return "API"
}

func (j *APIHost) IsInfinite() bool {
	return true
}

func (s *APIHost) Init(wm *WorkerMetrics) error {
	if s.Server == nil {
		s.Server = &NSpeedHTTPServer{Parameters: s.Parameters}
		return s.Server.Init(wm)
	}
	return nil
}

func (s *APIHost) Run(ctx context.Context) error {
	if s.Server == nil {
		return errors.New("API: no server")
	}
	return s.Server.Run(ctx)
}
