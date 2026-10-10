package nspeed

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"time"
)

var ErrUnknowCommand = fmt.Errorf("unknown API command")

// APIRun executes the specified API command (cmd) with arguments (args).
// Results are marshalled using the provided marshaller function.
func APIRun(ctx context.Context, globalOptions Options, cmd string, args []string, output io.Writer, who string, marshaller func(any) ([]byte, error)) ([]byte, error) {
	if cmd == "version" {
		v := struct {
			APIVersion     string
			NSpeedVersion  string
			RuntimeVersion string
		}{
			APIVersion:     APIVersion,
			NSpeedVersion:  Version,
			RuntimeVersion: runtime.Version(),
		}
		return marshaller(v)
	}

	if cmd == "time" {
		if len(args) == 0 {
			return marshaller(time.Now().Format(time.RFC1123))
		}
		if len(args) == 1 && args[0] == "unix" {
			return marshaller(time.Now().Unix())
		}
	}

	if cmd == "stats" {
		// /stats/info
		if len(args) >= 1 && args[0] == "info" {
			if len(args) == 1 {
				return MachineInfoJSON, nil
			}
			// stats/info/#field1#[/#field2/..]
			if len(args) >= 2 {
				fieldNames := args[1:]
				replyMap := make(map[string]any)
				for _, field := range fieldNames {
					if val, ok := MachineInfoMap[field]; ok {
						replyMap[field] = val
					} else {
						return nil, fmt.Errorf("unknown sub info: %s", field)
					}
				}
				return marshaller(replyMap)
			}
		}
		// stats/mem
		if len(args) >= 1 && args[0] == "mem" {
			// /stats/mem/gc will force a GC
			doGC := len(args) == 2 && args[1] == "gc"

			if len(args) == 1 || doGC {
				if doGC {
					runtime.GC()
				}
				MemStats.LoadStats()
				return marshaller(MemStats)
			}
			// stats/mem/#field#
			if len(args) >= 2 {
				return nil, fmt.Errorf("unknown sub mem: %s", args[1])
			}
		}
	}
	if cmd == "run" {
		// run [args]" => cli version => ./nspeed [args]

		if len(args) == 0 {
			return nil, fmt.Errorf("unknown run subcommand: %s", args)
		}

		jl := NewJoblist()
		name := fmt.Sprintf("from %s", who)
		err := jl.AddBatch(&Batch{Name: name})
		if err != nil {
			return nil, fmt.Errorf("error adding batch: %w", err)
		}
		err = ParseCommands(args, jl, output, true)
		if err != nil {
			return nil, fmt.Errorf("error parsing run command: %w", err)
		}

		results, err := RunJobs(ctx, jl, globalOptions, nil)
		if err != nil {
			return nil, fmt.Errorf("error running command: %w", err)
		}
		return marshaller(results) //results.MarshalJSON()
	}
	return nil, fmt.Errorf("%w: %s", ErrUnknowCommand, cmd)
}
