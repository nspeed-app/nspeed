package nspeed

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"nspeed.app/nspeed/pacing"
)

func TestL4Pacing(t *testing.T) {
	// Setup Server
	// We use L4Server via the interface or directly
	server := &L4Server{
		Parameters: &HostParameters{
			Protocol:    "TCP",
			Host:        "127.0.0.1",
			Port:        0, // Random port
			MaxDuration: 1 * time.Hour,
			IPVersion:   4,
		},
		JobList: NewJoblist(),
	}
	server.SetID("test-server")
	logger := zerolog.Nop()
	server.SetLogger(&logger)

	// Server Init
	opt := Options{
		IPVersion: 4,
	}
	if err := server.Init(opt); err != nil {
		t.Fatalf("Server Init failed: %v", err)
	}

	// Run Server
	ctxServer := t.Context()
	wgServer := &sync.WaitGroup{}
	wgServer.Add(1)

	go func() {
		_, err := server.Run(ctxServer, wgServer, opt)
		if err != nil && err != context.Canceled {
			// We can't t.Fatalf here easily as it's another goroutine, but we can log
			// In a real scenario we'd use a channel to communicate start failure
			fmt.Printf("Server failed: %v\n", err)
		}
	}()

	// Wait for server to be listening
	wgServer.Wait()

	// Check if server started successfully
	serverURL := server.GetURL()
	if serverURL == nil {
		t.Fatalf("Server failed to start (URL is nil)")
	}
	t.Logf("Server listening on %s", serverURL.String())

	// Client Setup
	// Create a schedule: hold 200ms, work 200ms
	holdDuration := 200 * time.Millisecond
	workDuration := 200 * time.Millisecond
	schedule, err := pacing.NewPacingSchedule(holdDuration, workDuration)
	if err != nil {
		t.Fatalf("failed to create schedule: %v", err)
	}

	// Resolve target
	targetURL, err := url.Parse(serverURL.String())
	if err != nil {
		t.Fatalf("failed to parse server url: %v", err)
	}

	job := &JobClient{
		JobData: JobData{
			// Initialize minimal required fields
		},
		Command:     "PUT",
		Protocol:    "TCP",
		Host:        "127.0.0.1", // Client source address (can be anything local)
		Target:      targetURL.String(),
		ContentSize: 1024 * 1024, // 1MB
		Schedule:    schedule,
		Timeout:     5 * time.Second,
	}
	job.SetLogger(&logger)
	job.targetURL = targetURL
	job.finalTarget = job.targetURL.String()

	// Dialer setup
	job.dialer = &net.Dialer{Timeout: 1 * time.Second}
	job.dialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
		return job.dialer.DialContext(ctx, network, address)
	}

	// Run Client
	ctxClient := context.Background()
	wgClient := &sync.WaitGroup{}
	wgClient.Add(1)

	startTime := time.Now()
	_, err = job.Run(ctxClient, wgClient, opt)
	if err != nil {
		t.Fatalf("Job Run failed: %v", err)
	}
	duration := time.Since(startTime)

	// Verification
	if duration < holdDuration {
		t.Errorf("expected duration >= %v, got %v", holdDuration, duration)
	}

	t.Logf("Total duration: %v", duration)
}
