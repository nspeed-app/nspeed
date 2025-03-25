package nspeed

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// run all bench, 1 second each
func TestBenchs(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.FatalLevel)
	for _, v := range benchKeys {
		t.Run(v, func(t *testing.T) {
			runBench(t, v, 1)
		})
	}
}

func TestAllBenchs(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	v := strings.Join(benchKeys, ",")
	fmt.Println(v)
	t.Run(v, func(t *testing.T) {
		runBench(t, v, 2)
	})
}

// run bench for dur seconds
func runBench(t *testing.T, bench string, dur int) {
	// t.Helper() - investigate
	jl := NewJoblist()
	err := jl.AddBatch(&Batch{Name: "test " + bench})
	if err != nil {
		t.Errorf("error adding batch: %s", err)
		return
	}
	args := strings.Split(fmt.Sprintf("b -m %d %s", dur, bench), " ")
	err = ParseCommands(args, jl, os.Stdout, true)
	if err != nil {
		t.Errorf("error parsing command: %s", err)
	}
	ctx := context.Background()
	globalOptions := Options{}
	results, err := RunJobs(ctx, jl, globalOptions, nil)
	if err != nil {
		t.Errorf("error running command: %s", err)
	}
	if results == nil {
		t.Errorf("no result for bench %s", bench)
	}
}
