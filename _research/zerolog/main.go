package main

import (
	"fmt"
	"os"
	"path"
	"runtime"

	"github.com/rs/zerolog"
)

type FilteredWriter struct {
	w     zerolog.LevelWriter
	level zerolog.Level
}

func (w *FilteredWriter) Write(p []byte) (n int, err error) {
	return w.w.Write(p)
}
func (w *FilteredWriter) WriteLevel(level zerolog.Level, p []byte) (n int, err error) {
	if level >= w.level {
		return w.w.WriteLevel(level, p)
	}
	return len(p), nil
}

type LineInfoHook struct{}

func (h LineInfoHook) Run(e *zerolog.Event, l zerolog.Level, msg string) {
	_, file, line, ok := runtime.Caller(2)
	if ok {
		e.Str("line", fmt.Sprintf("%s:%d", path.Base(file), line))
	} else {
		e.Str("line", "nok")
	}
}

func main() {

	fAll, _ := os.OpenFile("./test/test-all.log", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	fWarn, _ := os.OpenFile("./test/test-warn.log", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)

	errWriter := zerolog.MultiLevelWriter(fWarn)
	filteredWriter := &FilteredWriter{errWriter, zerolog.WarnLevel}
	w := zerolog.MultiLevelWriter(fAll, zerolog.ConsoleWriter{Out: os.Stdout}, filteredWriter)
	mainLog := zerolog.New(w).With().Str("some_key", "some_val").Timestamp().Logger()

	mainLog = mainLog.Hook(LineInfoHook{})

	mainLog.Debug().Msg("🛠️ Some trace")
	mainLog.Info().Msg("👌 Should work")
	mainLog.Warn().Msg("⚠️ My first warning")
	mainLog.Error().Msg("🛑 My last warning")
	mainLog.Fatal().Msg("💣 Kaboom!")
}
