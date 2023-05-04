package nspeed

import (
	"fmt"
	"io"
	"path/filepath"
	"runtime"

	"github.com/rs/zerolog"
)

//TODO: switch to slog

// slog doesn't define a TRACE and FATAL levels.
// to keep in sync with OpenTelemetry remove -9
// see https://pkg.go.dev/golang.org/x/exp/slog#Level
// and
// https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/logs/data-model.md#field-severitynumber
// https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/logs/data-model.md#displaying-severity
// var TraceLevel = slog.Level(v1.SeverityNumber_value["SEVERITY_NUMBER_TRACE"] - 9)
// var FatalLevel = slog.Level(v1.SeverityNumber_value["SEVERITY_NUMBER_FATAL"] - 9)

// see : https://github.com/rs/zerolog/issues/150#issuecomment-676859163
// enhancement: we add a bool ('exact'). if true we only log that level, otherwise we log that level and above

type FilteredWriter struct {
	w     zerolog.LevelWriter
	level zerolog.Level
	exact bool
}

func (w *FilteredWriter) Write(p []byte) (n int, err error) {
	return w.w.Write(p)
}
func (w *FilteredWriter) WriteLevel(level zerolog.Level, p []byte) (n int, err error) {
	if (w.exact && (level == w.level)) || (!w.exact && (level >= w.level)) {
		return w.w.WriteLevel(level, p)
	}
	return len(p), nil
}

// create a log writer limited to a level (exact = strict or at least)
func CreateFilteredWriter(w zerolog.LevelWriter, level zerolog.Level, exact bool) io.Writer {
	return &FilteredWriter{w: zerolog.MultiLevelWriter(w), level: level, exact: exact}
}

// should returns true if this logged would log at this lvl
func ShouldLog(l *zerolog.Logger, lvl zerolog.Level) bool {
	if l == nil {
		return false
	}
	return !(lvl < l.GetLevel() || lvl < zerolog.GlobalLevel())
}

type LineInfoHook struct {
	base string
	skip int
}

func (h LineInfoHook) Run(e *zerolog.Event, l zerolog.Level, msg string) {
	_, file, line, ok := runtime.Caller(h.skip)
	if !ok {
		return
	}
	relFile, err := filepath.Rel(h.base, file)
	if err != nil {
		relFile = file
	}
	e.Str("line", fmt.Sprintf("%s:%d", relFile, line))
}

func NewLineInfoHook(rel string) *LineInfoHook {
	lih := LineInfoHook{
		skip: 3,
	}
	_, file, _, ok := runtime.Caller(2)
	if !ok {
		return nil
	}
	lih.base = filepath.Join(filepath.Dir(file), rel)
	fmt.Println("file:", file)
	fmt.Println("base:", lih.base)
	return &lih
}
