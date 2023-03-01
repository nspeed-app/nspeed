package utils

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/quic-go/quic-go/logging"
	"github.com/rs/zerolog"
)

func QLogTracer(log *zerolog.Logger) func(logging.Perspective, []byte) io.WriteCloser {
	perspectiveNames := [...]string{"server", "client"}
	perspectiveName := func(p logging.Perspective) string {
		if p != 1 && p != 2 {
			return "unknown"
		}
		return perspectiveNames[p-1]
	}
	return func(p logging.Perspective, connID []byte) io.WriteCloser {
		filename := fmt.Sprintf("%s_%x.qlog", perspectiveName(p), connID)
		f, err := os.Create(filename)
		if err != nil {
			log.Fatal().Err(err).Send()
		}
		log.Trace().Msgf("Creating qlog file %s.\n", filename)
		return NewBufferedWriteCloser(bufio.NewWriter(f), f)
	}
}
