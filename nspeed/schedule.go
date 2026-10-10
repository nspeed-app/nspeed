package nspeed

import (
	"fmt"
	"strings"
	"time"
)

type Schedule struct {
	Wait    time.Duration
	Timeout time.Duration
}

// ParseSchedules parses a comma separated list of Schedule
//
// "0,2s,1s,1s" -> {0,2s},{1s,1s}
// "" is equivalent to {0,0} which means don't wait, dont timeout
// the last Timeout
func ParseSchedules(s string) ([]*Schedule, error) {
	if s == "" {
		return nil, nil
	}
	// "w0,t0,w1,t1,...,wN,tN"
	vals := strings.Split(s, ",")
	// [w0,t0,w1,t1,...,wN,tN]
	// if tN is missing, it's 0
	if len(vals)%2 == 1 {
		vals = append(vals, "0")
	}
	scheds := make([]*Schedule, 0, len(vals)/2)
	for i := range len(vals) / 2 {
		wait, err := time.ParseDuration(vals[i*2])
		if err != nil {
			return nil, fmt.Errorf("bad wait duration at position %d: %w", i*2+1, err)
		}
		timeOut, err := time.ParseDuration(vals[i*2+1])
		if err != nil {
			return nil, fmt.Errorf("bad timeout duration at position %d: %w", i*2+2, err)
		}
		scheds = append(scheds, &Schedule{Wait: wait, Timeout: timeOut})
	}
	return scheds, nil
}
