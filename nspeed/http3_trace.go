// Copyright (c) Jean-Francois Giorgi & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package nspeed

import (
	"context"
	"net"

	"github.com/quic-go/quic-go/logging"
	"nspeed.app/utils"
)

type quicTracer struct {
	logging.NullTracer
	wm *Metric
}

func (t *quicTracer) TracerForConnection(context.Context, logging.Perspective, logging.ConnectionID) logging.ConnectionTracer {
	return &quicConnectionTracer{wm: t.wm}
}

type quicConnectionTracer struct {
	logging.NullConnectionTracer
	wm *Metric
}

func (t *quicConnectionTracer) StartedConnection(local, remote net.Addr, srcConnID, destConnID logging.ConnectionID) {
	t.wm.Logger.Info().Msgf("QUIC: connected to %s from %s (ids=%s-%s)", remote, local, srcConnID, destConnID)
	if t.wm == nil {
		return
	}
	t.wm.Remote = remote.String()
	t.wm.Interface, t.wm.Gateway, t.wm.SourceAddr = utils.GetRoute(remote.String())

}

func (t *quicConnectionTracer) UpdatedMetrics(rttStats *logging.RTTStats, cwnd logging.ByteCount, bytesInFlight logging.ByteCount, packetsInFlight int) {
	// Trace might not occurs here but in qlog
	t.wm.Logger.Trace().Msgf("QUIC: rtt = %s - window=%d bif=%d pif=%d", rttStats.LatestRTT(), cwnd, bytesInFlight, packetsInFlight)
	if t.wm == nil {
		return
	}
	t.wm.AvgLatency = rttStats.SmoothedRTT()
}
