package nspeed

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
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

	j.Logger().Info().Msgf("L4 client dialing %s %s", network, j.targetURL.Host)

	conn, err := j.dialContext(ctx, network, j.targetURL.Host)
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

	j.wm.WriteStepSize = min(utils.MaxChunkSize, 1*1024*1024)
	chunk := utils.GetChunk(j.wm.WriteStepSize)
	j.wm.ContentWriteSize = j.ContentSize
	j.wm.TotalWrite = 0
	j.Logger().Info().Msgf("L4 client sending %d bytes to %s", j.wm.ContentWriteSize, j.targetURL)
	totalWrite := int64(0)
	for {
		n, err := j.wm.ReadN(j.wm.WriteStepSize)
		if err != nil { // ReadN only returns io.EOF
			j.Logger().Trace().Msgf("L4 client ReadN ending (err = %+v)", err)
			j.wm.Done = true
			break
		}
		nw, err := conn.Write(chunk[:min(j.wm.WriteStepSize, n)])
		totalWrite += int64(nw)
		j.Logger().Trace().Msgf("L4 client write %d bytes to %s (err=%s)", nw, j.targetURL, reflect.TypeOf(err))
		// possible error here:
		//  timeout because of Timeout or network error (both = a net.OpError error type)
		// either way we mark as 'cancelled' if timeout and return no error ?
		if err != nil {
			j.wm.TotalWrite = totalWrite
			var opError *net.OpError
			if errors.As(err, &opError) {
				if opError.Timeout() {
					j.wm.Cancelled = true
				}
				return j.wm, nil
			}
			j.wm.Error = fmt.Errorf("conn.Write error: %w", err)
			return j.wm, j.wm.Error
		}
	}
	j.wm.TotalWrite = totalWrite
	j.Logger().Info().Msgf("L4 client sent %d bytes to %s", j.wm.TotalWrite, j.targetURL)

	return j.wm, nil
}
