package nspeed

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"nspeed.app/metric"
	"nspeed.app/utils"
)

func (j *JobClient) L4Init() error {

	if j.Protocol != "tcp" {
		return fmt.Errorf("protocol %s not yet supported", j.Protocol)
	}
	return nil
}

func (j *JobClient) L4Run(ctx context.Context, wg *sync.WaitGroup) (*metric.Metric, error) {
	defer wg.Done()

	if j.Protocol != "tcp" {
		return nil, fmt.Errorf("protocol %s not yet supported", j.Protocol)
	}

	network := j.Protocol
	network = utils.AddIPVersionToNetwork(network, j.IPVersion, GlobalOptions.IPVersion)

	j.Logger().Info().Msgf("L4 client dialing %s %s", network, j.Host)

	conn, err := j.dialContext(ctx, network, j.Host)
	if err != nil {
		return nil, fmt.Errorf("dial error: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(j.Timeout)); err != nil {
		return nil, fmt.Errorf("SetDeadline error: %w", err)
	}

	if j.ContentSize == 0 {
		return nil, fmt.Errorf("no content to send")
	}

	// setup the metric
	j.wm = &metric.Metric{
		MetricsData: metric.MetricsData{
			Done:    false,
			Expired: false,
			Target:  j.GetTarget(),
			ID:      j.ID(),
			GroupID: j.GroupID(),
			Command: j.GoString(),
		},
		Tick: GlobalOptions.Tick,
	}
	j.wm.SetLogger(j.Logger())

	chunkSize := int64(1024 * 1024)
	chunk := utils.GetChunk(chunkSize)
	todo := j.ContentSize
	for {
		n, err := conn.Write(chunk[:min(chunkSize, todo)])
		if err != nil {
			if err == io.EOF {
				j.wm.Done = true
				break
			}
			j.wm.Error = err
			return j.wm, err
		}
		j.wm.WriteN(int64(n))
		todo -= int64(n)
	}
	return j.wm, nil
}
