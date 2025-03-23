// Copyright (c) Jean-Francois Giorgi & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package nspeed

import (
	"context"
	"net"
	"net/netip"

	"github.com/quic-go/quic-go/logging"
	"nspeed.app/helper/network"
	"nspeed.app/metrics"
)

type quicConnectionTracer struct {
	logging.ConnectionTracer
	wm *metrics.Metric
}

func (t *quicConnectionTracer) StartedConnection(local, remote net.Addr, srcConnID, destConnID logging.ConnectionID) {
	t.wm.Logger().Info().Msgf("QUIC: connected to %s from %s (ids=%s-%s)", remote, local, srcConnID, destConnID)
	if t.wm == nil {
		return
	}
	var err error
	t.wm.Remote, err = netip.ParseAddrPort(remote.String())
	if err != nil {
		t.wm.Logger().Debug().Err(err).Msgf("error parsing remote address: %s", remote.String())
	}

	t.wm.SourceAddr, err = netip.ParseAddrPort(remote.String())
	if err != nil {
		t.wm.Logger().Debug().Err(err).Msgf("error parsing local address:%s", remote.String())
	}
	t.wm.Interface, t.wm.Gateway, t.wm.RouteSourceAddr, _ = network.GetRoute(remote.String())
}

func (t *quicConnectionTracer) UpdatedMetrics(rttStats *logging.RTTStats, cwnd logging.ByteCount, bytesInFlight logging.ByteCount, packetsInFlight int) {
	// Trace might not occurs here but in qlog
	t.wm.Logger().Trace().Msgf("QUIC: rtt = %s - window=%d bif=%d pif=%d", rttStats.LatestRTT(), cwnd, bytesInFlight, packetsInFlight)
	if t.wm == nil {
		return
	}
	t.wm.AvgLatency = rttStats.SmoothedRTT()
}

func NewQuicTracer(wm *metrics.Metric) func(context.Context, logging.Perspective, logging.ConnectionID) *logging.ConnectionTracer {
	qct := quicConnectionTracer{wm: wm}
	qct.ConnectionTracer.StartedConnection = qct.StartedConnection
	qct.ConnectionTracer.UpdatedMetrics = qct.UpdatedMetrics

	f := func(context.Context, logging.Perspective, logging.ConnectionID) *logging.ConnectionTracer {
		return &qct.ConnectionTracer
	}
	return f
}
