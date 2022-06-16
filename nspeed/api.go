package nspeed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	_ "embed"

	"github.com/pkg/browser"
	"github.com/rs/zerolog/log"
	"nspeed.app/webview"
)

const APIVersion string = "1.0"

//go:embed API.md
var APIDocumentation []byte

type APIHost struct {
	Parameters    *HostParameters
	Server        *JobHTTPServer // underlying HTTP server
	WebStats      bool
	WebStatsOnly  bool
	WebStatsDebug bool
	OpenBrowser   bool
}

func (j *APIHost) GetTarget() string {
	return fmt.Sprintf("server %s:%d (IP version: %d)", j.Parameters.Host, j.Parameters.Port, j.Parameters.ForceIPVersion)
}

func (j *APIHost) Expand(jobs []Job, group int) []Job {
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

func (j *APIHost) PrintString(verbose bool) string {
	return "API " + net.JoinHostPort(j.Parameters.Host, strconv.Itoa(j.Parameters.Port))
}

func (j *APIHost) IsInfinite() bool {
	return true
}

func (j *APIHost) GetMetric() (NetMetric, error) {
	return NetMetric{}, fmt.Errorf("not yet implemented")
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
	if !s.WebStatsOnly {
		s.Server.Mux.Handle("/api/v1/", http.StripPrefix("/api/v1/", http.HandlerFunc(s.apiHandler)))
		s.Server.Mux.Handle("/api/", http.HandlerFunc(s.apiHelpHandler))
	}
	if s.WebStats {
		mgr := webview.NewViewManager(time.Duration(GlobalOptions.ProgressInterval)*time.Second, "/rt", s.WebStatsDebug)
		mgr.RegisterRoutes(s.Server.Mux)
		fmt.Printf("real time web stats at http://%s/rt \n", s.Server.Server.Addr)
	}
	return nil
}

func (s *APIHost) Run(ctx context.Context) error {
	if s.Server == nil {
		return errors.New("API: no server")
	}
	// wait 1 sonde and open a browser
	if s.OpenBrowser {
		go func() {
			// todo: change this sleep to a signal or test of the url  ?
			time.Sleep(1 * time.Second)
			browser.OpenURL(fmt.Sprintf("http://localhost:%d/rt", s.Parameters.Port))
		}()
	}
	return s.Server.Run(ctx)
}

func (h *APIHost) apiHelpHandler(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write(APIDocumentation)
}

func (h *APIHost) apiHandler(w http.ResponseWriter, r *http.Request) {

	method := r.Method
	if method == "" {
		method = "GET"
	}
	path := r.URL.Path
	log.Warn().Msgf("apiHandler: %s - %s from %s", method, path, r.RemoteAddr)

	if method != "GET" {
		http.Error(w, "no such method", http.StatusNotImplemented)
		return
	}

	// parse the URL path, cmd is first word then everywthing after the first '/' is an array of parameters
	// "cmd/param1/param1/param1"
	args := strings.Split(path, "/")
	cmd := args[0]
	args = args[1:]

	//todo: refactor using a func per command or a global array of command/func
	//goal = allow adding new commands without editing this function

	if cmd == "help" || cmd == "" {
		h.apiHelpHandler(w, r)
		return
	}

	if cmd == "headers" {
		for name, headers := range r.Header {
			for _, h := range headers {
				fmt.Fprintf(w, "%v: %v\n", name, h)
			}
		}
		fmt.Fprint(w, "\n")
		return
	}

	if cmd == "version" {
		fmt.Fprintf(w, "API version %s\nNSpeed version %s\nGo version: %s\n", APIVersion, Version, runtime.Version())
		return
	}

	if cmd == "ip" {
		if len(args) == 0 {
			fmt.Fprintf(w, "%s\n", r.RemoteAddr)
			return
			//todo: add geoip query https://github.com/oschwald/maxminddb-golang
		}
	}

	if cmd == "time" {
		if len(args) == 0 {
			fmt.Fprintf(w, "The server time is: %s\n", time.Now().Format(time.RFC1123))
			return
		}
		if len(args) == 1 && args[0] == "unix" {
			fmt.Fprintf(w, "%d", time.Now().Unix())
			return

		}
	}

	if cmd == "stats" {
		// /stats/info
		if len(args) >= 1 && args[0] == "info" {
			if len(args) == 1 {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, "%s\n", MachineInfoJSON)
				return
			}
			// stats/info/#field1#[/#field2/..]
			if len(args) >= 2 {
				fieldNames := args[1:]
				replyMap := make(map[string]any)
				for _, field := range fieldNames {
					fmt.Printf("checking field %s\n", field)
					if val, ok := MachineInfoMap[field]; ok {
						replyMap[field] = val
					} else {
						http.Error(w, fmt.Sprintf("unknown sub info: %s", field), http.StatusBadRequest)
						return
					}
				}
				_ = json.NewEncoder(w).Encode(replyMap)
				return
			}
		}
		// stats/mem
		if len(args) >= 1 && args[0] == "mem" {
			// /stats/mem/gc will force a GC
			doGC := len(args) == 2 && args[1] == "gc"

			if len(args) == 1 || doGC {
				w.Header().Set("Content-Type", "application/json")
				if doGC {
					runtime.GC()
				}
				MemStats.LoadStats()
				jw := json.NewEncoder(w)
				jw.SetIndent("", "  ")
				_ = jw.Encode(MemStats)
				return
			}
			// stats/mem/#field#
			if len(args) >= 2 {
				http.Error(w, fmt.Sprintf("unknown sub mem: %s", args[1]), http.StatusBadRequest)
				return
			}
		}
	}
	if cmd == "run" {
		// /api/v1/run?cmd="...." <=> nspeed ....

		if len(args) != 0 {
			http.Error(w, fmt.Sprintf("unknown 'run' subcommand: %s", args), http.StatusBadRequest)
			return
		}

		log.Warn().Msgf("apiHandler: run: %s", args)

		a, ok := r.URL.Query()["args"]

		if !ok {
			http.Error(w, "'run' command missing 'args' parameter", http.StatusBadRequest)
			return
		}

		var args []string
		for i := range a {
			log.Warn().Msgf("apiHandler: run: %d - %s", i, a[i])
			args = append(args, strings.Fields(a[i])...)
		}

		jobs, err := ParseCommands(args, w, true)
		if err != nil {
			fmt.Fprintf(w, "error parsing run command: %s\n", err)
			return
		}
		log.Warn().
			Str("remoteAddress", r.RemoteAddr).
			Str("command", "run").
			Strs("args", args).
			Send()

		f, okToFlush := w.(http.Flusher)
		wcWorkers, err := RunJobs(jobs, GlobalOptions, r.Context(), func(wms []*Metrics) {
			log.Debug().Msgf("api: progress called")
			fmt.Fprintf(w, "api @%s - progress %s: %d\n", r.Host, time.Now().String(), len(wms))
			if okToFlush {
				f.Flush()
			}
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("error running command: %s", err), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "api @%s - ended: %s: %d\n", r.Host, time.Now().String(), len(wcWorkers))
		for _, wm := range wcWorkers {
			fmt.Fprintf(w, "api @%s - result:", r.Host)
			wm.PrintFinal(w, true)
		}
		//todo: send results back
		return
	}
	http.Error(w, "unknown API command", http.StatusNotFound)
}
