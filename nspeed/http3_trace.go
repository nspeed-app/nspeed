// Copyright (c) Jean-Francois Giorgi & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package nspeed

import (
	"context"

	"nspeed/metrics"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/qlog"
	"github.com/quic-go/quic-go/qlogwriter"
	"nspeed.app/network"
)

type quicConnectionTracer struct {
	qlogwriter.Trace
	wm *metrics.Metric
}

func (qct *quicConnectionTracer) AddProducer() qlogwriter.Recorder {
	return &quicConnectionRecorder{wm: qct.wm}
}
func (qct *quicConnectionTracer) SupportsSchemas(schema string) bool {
	return true //FIXME
}

type quicConnectionRecorder struct {
	wm *metrics.Metric
}

func (qr *quicConnectionRecorder) RecordEvent(e qlogwriter.Event) {
	// fmt.Println("event", e.Name())
	switch evt := e.(type) {
	case qlog.StartedConnection:
		qr.wm.SourceAddr = evt.Remote.IPv6 //fixme
		qr.wm.Interface, qr.wm.Gateway, qr.wm.RouteSourceAddr, _ = network.GetRouteFromAddrPort(qr.wm.SourceAddr.String())
	case qlog.MetricsUpdated:
		// Trace might not occurs here but in qlog
		qr.wm.Logger().Trace().Msgf("QUIC: rtt = %d - window=%d bif=%d pif=%d",
			evt.MinRTT, evt.CongestionWindow,
			evt.BytesInFlight,
			evt.PacketsInFlight)
		qr.wm.AvgLatency = evt.SmoothedRTT
	case qlog.KeyUpdated:
		qr.wm.Logger().Trace().Msgf("key phase: %d", evt.KeyPhase)
	}
}

func (qr *quicConnectionRecorder) Close() error {
	return nil
}

// func (t *quicConnectionTracer) StartedConnection(local, remote net.Addr, srcConnID, destConnID logging.ConnectionID) {
// 	t.wm.Logger().Info().Msgf("QUIC: connected to %s from %s (ids=%s-%s)", remote, local, srcConnID, destConnID)
// 	if t.wm == nil {
// 		return
// 	}
// 	var err error
// 	t.wm.Remote, err = netip.ParseAddrPort(remote.String())
// 	if err != nil {
// 		t.wm.Logger().Debug().Err(err).Msgf("error parsing remote address: %s", remote.String())
// 	}

func NewQuicTracer(wm *metrics.Metric) func(ctx context.Context, client bool, id quic.ConnectionID) qlogwriter.Trace {
	// func(context.Context, logging.Perspective, logging.ConnectionID) *logging.ConnectionTracer {
	qct := quicConnectionTracer{wm: wm}
	// qct.ConnectionTracer.StartedConnection = qct.StartedConnection
	// qct.ConnectionTracer.UpdatedMetrics = qct.UpdatedMetrics

	f := func(ctx context.Context, client bool, id quic.ConnectionID) qlogwriter.Trace {
		return &qct
	}
	return f
}
