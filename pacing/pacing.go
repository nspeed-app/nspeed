// Copyright (c) Jean-Francois Giorgi & AUTHORS
// part of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause

package pacing

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// Schedule represents alternating hold and work durations
type Schedule struct {
	phases []phase
}

type phase struct {
	hold     bool // true for hold/wait, false for work/download
	duration time.Duration
}

func (ps *Schedule) String() string {
	var parts []string
	for _, p := range ps.phases {
		parts = append(parts, p.duration.String())
	}
	if len(parts) > 0 && parts[len(parts)-1] == "0s" {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(parts, ",")
}

// NewPacingSchedule creates a schedule from alternating h,w durations
//
// Example:
//
//	NewPacingSchedule(2*time.Second, 5*time.Second, 1*time.Second, 3*time.Second)
//
// means: hold 2s, work 5s, hold 1s, work 3s
//
// If the number of durations is odd, the last work phase is assumed to be infinite (duration 0).
func NewPacingSchedule(durations ...time.Duration) (*Schedule, error) {
	if len(durations)%2 != 0 {
		durations = append(durations, 0)
	}

	ps := &Schedule{
		phases: make([]phase, len(durations)),
	}

	for i := 0; i < len(durations); i += 2 {
		ps.phases[i] = phase{hold: true, duration: durations[i]}
		ps.phases[i+1] = phase{hold: false, duration: durations[i+1]}
	}

	return ps, nil
}

// ParsePacingSchedule parses a comma-separated string of durations into a PacingSchedule
// Example: "2s,5s,1s,3s" or "2s,5s,1s" (last work phase infinite)
func ParsePacingSchedule(scheduleStr string) (*Schedule, error) {
	parts := strings.Split(scheduleStr, ",")
	var durations []time.Duration
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		d, err := time.ParseDuration(trimmed)
		if err != nil {
			return nil, fmt.Errorf("invalid duration %q: %w", trimmed, err)
		}
		durations = append(durations, d)
	}
	return NewPacingSchedule(durations...)
}

// Pacer manages the timing of phases
type Pacer struct {
	ctx      context.Context
	schedule *Schedule
	phaseIdx int
	phaseEnd time.Time
	started  bool
}

func NewPacer(ctx context.Context, schedule *Schedule) *Pacer {
	return &Pacer{
		ctx:      ctx,
		schedule: schedule,
		phaseIdx: 0,
	}
}

// Wait blocks until the current hold phase is over, or returns immediately if in work phase.
// It handles phase transitions and cancellation.
func (p *Pacer) Wait() error {
	if !p.started {
		p.started = true
		p.startPhase()
	}

	for {
		// Check cancellation
		if err := p.ctx.Err(); err != nil {
			return err
		}

		// Check if we've exhausted all phases
		if p.phaseIdx >= len(p.schedule.phases) {
			return nil
		}

		currentPhase := p.schedule.phases[p.phaseIdx]

		// Special case for 0 duration
		if currentPhase.duration == 0 {
			if currentPhase.hold {
				// 0 duration hold is instant -> move to next phase
				p.phaseIdx++
				if p.phaseIdx >= len(p.schedule.phases) {
					return nil
				}
				p.startPhase()
				continue
			}
			// 0 duration work is infinite
			return nil
		}

		now := time.Now()

		// If phase has ended, move to next
		if now.After(p.phaseEnd) {
			p.phaseIdx++
			if p.phaseIdx >= len(p.schedule.phases) {
				return nil
			}
			p.startPhase()
			continue
		}

		// If in hold phase, sleep until phase ends
		if currentPhase.hold {
			sleepDuration := p.phaseEnd.Sub(now)
			select {
			case <-time.After(sleepDuration):
				p.phaseIdx++
				if p.phaseIdx >= len(p.schedule.phases) {
					return nil
				}
				p.startPhase()
				continue
			case <-p.ctx.Done():
				return p.ctx.Err()
			}
		}

		// In work phase
		return nil
	}
}

func (p *Pacer) startPhase() {
	p.phaseEnd = time.Now().Add(p.schedule.phases[p.phaseIdx].duration)
}

// PacedReader wraps an io.Reader and applies pacing according to schedule
type PacedReader struct {
	r     io.Reader
	pacer *Pacer
}

func NewPacedReader(ctx context.Context, r io.Reader, schedule *Schedule) *PacedReader {
	return &PacedReader{
		r:     r,
		pacer: NewPacer(ctx, schedule),
	}
}

func (pr *PacedReader) Read(p []byte) (n int, err error) {
	if err := pr.pacer.Wait(); err != nil {
		return 0, err
	}
	return pr.r.Read(p)
}

// PacedWriter wraps an io.Writer and applies pacing according to schedule
type PacedWriter struct {
	w     io.Writer
	pacer *Pacer
}

func NewPacedWriter(ctx context.Context, w io.Writer, schedule *Schedule) *PacedWriter {
	return &PacedWriter{
		w:     w,
		pacer: NewPacer(ctx, schedule),
	}
}

func (pw *PacedWriter) Write(p []byte) (n int, err error) {
	if err := pw.pacer.Wait(); err != nil {
		return 0, err
	}
	return pw.w.Write(p)
}
