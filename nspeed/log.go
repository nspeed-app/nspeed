package nspeed

import (
	"io"

	"github.com/rs/zerolog"
)

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
