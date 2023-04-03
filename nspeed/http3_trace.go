// Copyright (c) Jean-Francois Giorgi & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package nspeed

import (
	"context"
	"net"

	"github.com/quic-go/quic-go/logging"
)

type quicTracer struct {
	logging.NullTracer
	wm *Metric
}

func (t *quicTracer) TracerForConnection(context.Context, logging.Perspective, logging.ConnectionID) logging.ConnectionTracer {
	return &quicConnectionTracer{M: t.wm}
}

type quicConnectionTracer struct {
	logging.NullConnectionTracer
	M *Metric
}

func (n *quicConnectionTracer) StartedConnection(local, remote net.Addr, srcConnID, destConnID logging.ConnectionID) {
	n.M.Logger.Debug().Msgf("QUIC: connected to %s", remote)
	if n.M == nil {
		return
	}
	n.M.Remote = remote.String()
}

func (n *quicConnectionTracer) UpdatedMetrics(rttStats *logging.RTTStats, cwnd logging.ByteCount, bytesInFlight logging.ByteCount, packetsInFlight int) {
	n.M.Logger.Debug().Msgf("QUIC: rtt = %s", rttStats.LatestRTT())
	if n.M == nil {
		return
	}
	n.M.AvgLatency = rttStats.SmoothedRTT()
}
