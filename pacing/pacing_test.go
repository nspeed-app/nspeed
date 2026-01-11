package pacing

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParsePacingSchedule(t *testing.T) {
	tests := []struct {
		name        string
		scheduleStr string
		want        *PacingSchedule
		wantErr     bool
	}{
		{
			name:        "Valid schedule",
			scheduleStr: "2s,5s,1s,3s",
			want: &PacingSchedule{
				phases: []phase{
					{hold: true, duration: 2 * time.Second},
					{hold: false, duration: 5 * time.Second},
					{hold: true, duration: 1 * time.Second},
					{hold: false, duration: 3 * time.Second},
				},
			},
			wantErr: false,
		},
		{
			name:        "Valid schedule with spaces",
			scheduleStr: " 2s , 5s , 1s , 3s ",
			want: &PacingSchedule{
				phases: []phase{
					{hold: true, duration: 2 * time.Second},
					{hold: false, duration: 5 * time.Second},
					{hold: true, duration: 1 * time.Second},
					{hold: false, duration: 3 * time.Second},
				},
			},
			wantErr: false,
		},
		{
			name:        "Empty string",
			scheduleStr: "",
			want: &PacingSchedule{
				phases: []phase{},
			},
			wantErr: false,
		},
		{
			name:        "Odd number of durations (infinite last)",
			scheduleStr: "2s,5s,1s",
			want: &PacingSchedule{
				phases: []phase{
					{hold: true, duration: 2 * time.Second},
					{hold: false, duration: 5 * time.Second},
					{hold: true, duration: 1 * time.Second},
					{hold: false, duration: 0},
				},
			},
			wantErr: false,
		},
		{
			name:        "Invalid duration format",
			scheduleStr: "2s,invalid,1s,3s",
			want:        nil,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePacingSchedule(tt.scheduleStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePacingSchedule() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParsePacingSchedule() = %v, want %v", got, tt.want)
			}
		})
	}
}

type noopWriter struct{}

func (nw *noopWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func TestPacedReader_Cancel(t *testing.T) {
	// Schedule: hold 1h, work 1s
	schedule, err := NewPacingSchedule(1*time.Hour, 1*time.Second)
	if err != nil {
		t.Fatalf("NewPacingSchedule failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	r := strings.NewReader("hello")
	pr := NewPacedReader(ctx, r, schedule)

	var wg sync.WaitGroup
	wg.Add(1)
	errChan := make(chan error)
	go func() {
		wg.Done()
		buf := make([]byte, 5)
		_, err := pr.Read(buf)
		errChan <- err
	}()

	// Allow goroutine to start
	wg.Wait()
	// Yield to allow Read to potentially start
	time.Sleep(1 * time.Millisecond)
	cancel()

	select {
	case err := <-errChan:
		if err != context.Canceled {
			t.Errorf("Read() error = %v, want %v", err, context.Canceled)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Read() timed out waiting for cancellation")
	}
}

func TestPacedWriter_Cancel(t *testing.T) {
	// Schedule: hold 1h, work 1s
	schedule, err := NewPacingSchedule(1*time.Hour, 1*time.Second)
	if err != nil {
		t.Fatalf("NewPacingSchedule failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	w := &noopWriter{}
	pw := NewPacedWriter(ctx, w, schedule)

	var wg sync.WaitGroup
	wg.Add(1)
	errChan := make(chan error)
	go func() {
		wg.Done()
		_, err := pw.Write([]byte("hello"))
		errChan <- err
	}()

	// Allow goroutine to start
	wg.Wait()
	// Yield to allow Write to potentially start
	time.Sleep(1 * time.Millisecond)
	cancel()

	select {
	case err := <-errChan:
		if err != context.Canceled {
			t.Errorf("Write() error = %v, want %v", err, context.Canceled)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Write() timed out waiting for cancellation")
	}
}

func TestPacer_ZeroDurations(t *testing.T) {
	// Schedule: hold 0 (instant), work 0 (infinite)
	schedule, err := NewPacingSchedule(0, 0)
	if err != nil {
		t.Fatalf("NewPacingSchedule failed: %v", err)
	}

	ctx := context.Background()
	pacer := NewPacer(ctx, schedule)

	// First call -> Wait should process hold(0) effectively skipping it, 
	// then hit work(0) which returns nil immediately (infinite work).
	start := time.Now()
	if err := pacer.Wait(); err != nil {
		t.Errorf("Wait() error = %v, want nil", err)
	}
	if time.Since(start) > 10*time.Millisecond {
		t.Error("Wait() took too long for 0 hold duration")
	}

	// Verify we are in work phase (implied by Wait returning nil immediately + 0 duration work)
	// Internally phaseIdx should still point to work phase (1) because infinite work never ends
	// But we can't easily check internal state without reflection or exporting vars.
	// We can check that subsequent calls also return immediately
	if err := pacer.Wait(); err != nil {
		t.Errorf("Subsequent Wait() error = %v, want nil", err)
	}
}
