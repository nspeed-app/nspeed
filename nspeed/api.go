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
	"sync"
	"time"

	_ "embed"

	"github.com/rs/zerolog"
	"nspeed.app/utils"
	"nspeed.app/webview"
)

const APIVersion string = "1.0"
const StatsPath = "/rt"

//go:embed API.md
var APIDocumentation []byte

type APIHost struct {
	JobData
	Parameters        *HostParameters
	Server            *JobHTTPServer // underlying endpoint
	WebStats          bool
	WebStatsOnly      bool
	WebStatsDebug     bool
	OpenBrowser       bool
	WebStatsInterface []string
}

func (j *APIHost) GetTarget() string {
	return fmt.Sprintf("server %s:%d (IP version: %d)", j.Parameters.Host, j.Parameters.Port, j.Parameters.ForceIPVersion)
}

func (j *APIHost) GoString() string {
	if zerolog.GlobalLevel() > zerolog.InfoLevel {
		return fmt.Sprintf("API server port: %d", j.Parameters.Port)
	}
	return fmt.Sprintf("API {Address: %q, port: %d, IPversion: %d, UseTLS: %t}",
		j.Parameters.Host,
		j.Parameters.Port,
		j.Parameters.ForceIPVersion,
		j.Parameters.ServeTLS,
	)
}

func (j *APIHost) PrintString(_, _ bool) string {
	return "API " + net.JoinHostPort(j.Parameters.Host, strconv.Itoa(j.Parameters.Port))
}

func (j *APIHost) IsInfinite() bool {
	return true
}

func (j *APIHost) Init() error {
	if j.Parameters == nil {
		j.Parameters = NewHostParameters()
		j.Parameters.APIOnly = true
	}
	if j.Server == nil {
		j.Parameters.APIOnly = true
		j.Server = &JobHTTPServer{Parameters: j.Parameters}
		err := j.Server.Init()
		if err != nil {
			return err
		}
	}
	if j.Server.Mux == nil {
		return errors.New("API: no Mux in server")
	}
	if !j.WebStatsOnly {
		j.Server.Mux.Handle("/api/v1/", http.StripPrefix("/api/v1/", http.HandlerFunc(j.apiHandler)))
		j.Server.Mux.Handle("/api/", http.HandlerFunc(j.apiHelpHandler))
	}
	if j.WebStats {
		mgr, err := webview.NewViewManager(GlobalOptions.ProgressInterval, StatsPath, j.WebStatsInterface, j.WebStatsDebug)
		if err != nil {
			return err
		}
		mgr.RegisterRoutes(j.Server.Mux)
	}
	return nil
}

func (j *APIHost) Run(ctx context.Context, wg *sync.WaitGroup) (*Metric, error) {
	if j.Server == nil {
		return nil, errors.New("API: no server")
	}
	j.Server.RunHook = func(c context.Context) error {
		return j.PostRun()
	}
	return j.Server.Run(ctx, wg)
}

func (j *APIHost) apiHelpHandler(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write(APIDocumentation)
}

func (j *APIHost) apiHandler(w http.ResponseWriter, r *http.Request) {

	method := r.Method
	if method == "" {
		method = "GET"
	}
	path := r.URL.Path
	j.Logger().Warn().Msgf("apiHandler: %s - %s from %s", method, path, r.RemoteAddr)

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
		j.apiHelpHandler(w, r)
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

		j.Logger().Warn().Msgf("apiHandler: run: %s", args)

		a, ok := r.URL.Query()["args"]

		if !ok {
			http.Error(w, "'run' command missing 'args' parameter", http.StatusBadRequest)
			return
		}

		var args []string
		for i := range a {
			j.Logger().Warn().Msgf("apiHandler: run: %d - %s", i, a[i])
			args = append(args, strings.Fields(a[i])...)
		}

		jl := NewJoblist()
		name := fmt.Sprintf("from %s", r.RemoteAddr)
		err := jl.AddBatch(&Batch{Name: name})
		if err != nil {
			fmt.Fprintf(w, "error adding batch: %s\n", err)
			return
		}
		err = ParseCommands(args, jl, w, true)
		if err != nil {
			fmt.Fprintf(w, "error parsing run command: %s\n", err)
			return
		}
		j.Logger().Warn().
			Str("remoteAddress", r.RemoteAddr).
			Str("command", "run").
			Strs("args", args).
			Send()

		//f, okToFlush := w.(http.Flusher)

		wcWorkers, err := RunJobs(r.Context(), jl, GlobalOptions, nil)
		if err != nil {
			http.Error(w, fmt.Sprintf("error running command: %s", err), http.StatusInternalServerError)
			return
		}
		json, err := json.Marshal(wcWorkers)
		if err != nil {
			fmt.Fprintf(w, "error parsing json reply from %s: %s\n", r.RemoteAddr, err)
		}
		fmt.Fprintf(w, "%s", json)
		return
	}
	http.Error(w, "unknown API command", http.StatusNotFound)
}

// PostRun is called when the endpoint server is running
func (j *APIHost) PostRun() error {
	url := j.Server.URL.JoinPath(StatsPath)
	if j.WebStats {
		j.Logger().Warn().Msgf("real time web stats at %s", url)
	}
	if j.WebStatsDebug {
		// we should get the route from wher it's defined
		// add a link in rt to this ?
		j.Logger().Warn().Msgf("Go runtime stats at %s", j.Server.URL.JoinPath(webview.GoStatsRoute))
		if !j.WebStats {
			url = j.Server.URL.JoinPath(webview.GoStatsRoute)
		}
	}
	if j.OpenBrowser {
		j.Logger().Warn().Msgf("opening browser at %s", url)
		return utils.OpenBrowser(url.String(), nil, nil)
	}
	return nil
}
