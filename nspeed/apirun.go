package nspeed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"
)

func APIRun(ctx context.Context, globalOptions Options, cmd string, args []string, w http.ResponseWriter, who string) error {
	fmt.Println("run from", who, "cmd=", cmd, "args=", args)
	if cmd == "help" || cmd == "" {
		w.Write(APIDocumentation)
		return nil
	}
	if cmd == "version" {
		fmt.Fprintf(w, "API version %s\nNSpeed version %s\nGo version: %s\n", APIVersion, Version, runtime.Version())
		return nil
	}

	if cmd == "time" {
		if len(args) == 0 {
			fmt.Fprintf(w, "The server time is: %s\n", time.Now().Format(time.RFC1123))
			return nil
		}
		if len(args) == 1 && args[0] == "unix" {
			fmt.Fprintf(w, "%d", time.Now().Unix())
			return nil
		}
	}

	if cmd == "stats" {
		// /stats/info
		if len(args) >= 1 && args[0] == "info" {
			if len(args) == 1 {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, "%s\n", MachineInfoJSON)
				return nil
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
						return nil
					}
				}
				_ = json.NewEncoder(w).Encode(replyMap)
				return nil
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
				return nil
			}
			// stats/mem/#field#
			if len(args) >= 2 {
				http.Error(w, fmt.Sprintf("unknown sub mem: %s", args[1]), http.StatusBadRequest)
				return nil
			}
		}
	}
	if cmd == "run" {
		// run [args]" => cli version => ./nspeed [args]

		if len(args) == 0 {
			http.Error(w, fmt.Sprintf("unknown 'run' subcommand: %s", args), http.StatusBadRequest)
			return nil
		}

		jl := NewJoblist()
		name := fmt.Sprintf("from %s", who)
		err := jl.AddBatch(&Batch{Name: name})
		if err != nil {
			fmt.Fprintf(w, "error adding batch: %s\n", err)
			return nil
		}
		err = ParseCommands(args, jl, w, true)
		if err != nil {
			fmt.Fprintf(w, "error parsing run command: %s\n", err)
			return nil
		}

		results, err := RunJobs(ctx, jl, globalOptions, nil)
		if err != nil {
			http.Error(w, fmt.Sprintf("error running command: %s", err), http.StatusInternalServerError)
			return nil
		}
		json, err := results.MarshalJSON()
		if err != nil {
			fmt.Fprintf(w, "error parsing json reply from %s: %s\n", who, err)
		}
		fmt.Fprintf(w, "%s", json)
		return nil
	}
	http.Error(w, "unknown API command", http.StatusNotFound)
	return nil
}
