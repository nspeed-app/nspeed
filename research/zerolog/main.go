package main

import (
	"os"

	"github.com/rs/zerolog"
)

func main() {
	fAll, _ := os.OpenFile("./test-all.log", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	fWarn, _ := os.OpenFile("./test-warn.log", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)

	rootLogger := zerolog.New(fAll).With().Str("some_key", "some_val").Timestamp().Logger()
	warnLogger := zerolog.New(fWarn).With().Str("additional", "context").Logger().Level(zerolog.WarnLevel)
	consoleLogger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "15:04:05"})

	w := zerolog.MultiLevelWriter(rootLogger, warnLogger, consoleLogger)
	mainLog := zerolog.New(w)
	mainLog.Debug().Msg("🛠️ Some trace")
	mainLog.Info().Msg("👌 Should work")
	mainLog.Warn().Msg("⚠️ My first warning")
	mainLog.Error().Msg("🛑 My last warning")
	mainLog.Fatal().Msg("💣 Kaboom!")
}
