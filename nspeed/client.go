package nspeed

import (
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"nspeed.app/metric"
	"nspeed.app/utils"
)

type L4Client struct {
	TLSSettings
	JobData
	Protocol    string
	Command     string
	Repeat      int
	IPVersion   utils.IPVersion
	Host        string
	Target      string
	Timeout     time.Duration
	DialTimeout time.Duration
	ContentSize int64
	Delay       time.Duration
	// private
	wm *metric.Metric
}

func (j *L4Client) GetTarget() string {
	return fmt.Sprintf("%s client %s (IP version: %d)", j.Protocol, j.Host, j.IPVersion)
}

func (j *L4Client) GoString() string {
	if zerolog.GlobalLevel() > zerolog.InfoLevel {
		return fmt.Sprintf("%s server %s", j.Protocol, j.Host)
	}
	return fmt.Sprintf("%s server {Address: %q, IPversion: %d, max_size: %s, max_request, max duration: %v, UseTLS: %t}",
		j.Protocol,
		j.Host,
		j.IPVersion,
		utils.ByteCountDecimal(j.ContentSize)+"B",
		j.Timeout,
		j.ServeTLS,
	)
}

func (j *L4Client) PrintString(_, _ bool) string {
	return j.Protocol + " server " + j.ID()
}

func (j *L4Client) IsInfinite() bool {
	return true
}
