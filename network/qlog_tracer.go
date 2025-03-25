package network

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/logging"
	"github.com/quic-go/quic-go/qlog"
	"github.com/rs/zerolog"
)

func GenerateQLogTracer(log *zerolog.Logger) func(ctx context.Context, p logging.Perspective, connID quic.ConnectionID) *logging.ConnectionTracer {
	QLogTracer := func(ctx context.Context, p logging.Perspective, connID quic.ConnectionID) *logging.ConnectionTracer {
		perspectiveNames := [...]string{"server", "client"}
		perspectiveName := func(p logging.Perspective) string {
			if p != 1 && p != 2 {
				return "unknown"
			}
			return perspectiveNames[p-1]
		}
		filename := fmt.Sprintf("%s_%x.qlog", perspectiveName(p), connID)
		f, err := os.Create(filename)
		if err != nil {
			log.Fatal().Err(err).Send()
		}
		log.Trace().Msgf("Creating qlog file %s.\n", filename)
		return qlog.NewConnectionTracer(NewBufferedWriteCloser(bufio.NewWriter(f), f), p, connID)
	}
	return QLogTracer
}
