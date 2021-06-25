package nspeed

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
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

const APIVersion string = "1.0"

type APIHost struct {
	Parameters *HostParameters
	Server     *JobHTTPServer // underlying HTTP server
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
	return "API " + net.JoinHostPort(j.Parameters.Host, strconv.Itoa(j.Parameters.Port))
}

func (j *APIHost) IsInfinite() bool {
	return true
}

func (s *APIHost) Init(wm *Metrics) error {
	if s.Parameters == nil {
		s.Parameters = NewHostParameters()
		s.Parameters.APIOnly = true
	}
	if s.Server == nil {
		s.Parameters.APIOnly = true
		s.Server = &JobHTTPServer{Parameters: s.Parameters}
		err := s.Server.Init(wm)
		if err != nil {
			return err
		}
	}
	if s.Server.Mux == nil {
		return errors.New("API: no Mux in server")
	}
	s.Server.Mux.Handle("/api/", http.StripPrefix("/api/", http.HandlerFunc(s.apiHandler)))
	return nil
}

func (s *APIHost) Run(ctx context.Context) error {
	if s.Server == nil {
		return errors.New("API: no server")
	}
	return s.Server.Run(ctx)
}

func (h *APIHost) apiHandler(w http.ResponseWriter, r *http.Request) {

	method := r.Method
	if method == "" {
		method = "GET"
	}
	args := r.URL.Path
	log.Warn().Msgf("apiHandler: %s - %s from %s", method, args, r.RemoteAddr)

	if method != "GET" {
		http.Error(w, "no such method", http.StatusNotImplemented)
		return
	}

	if args == "headers" {
		for name, headers := range r.Header {
			for _, h := range headers {
				fmt.Fprintf(w, "%v: %v", name, h)
			}
		}
		fmt.Fprint(w, "\n")
		return
	}

	if args == "version" {
		fmt.Fprintf(w, "API version %s\nNSpeed version %s\n", APIVersion, Version)
		return
	}

	if args == "ip" {
		fmt.Fprintf(w, "%s\n", r.RemoteAddr)
		return
	}

	if args == "time" {
		fmt.Fprintf(w, "The server time is: %s\n", time.Now().Format(time.RFC1123))
		return
	}

	http.Error(w, "unknown API command", http.StatusNotFound)
}
