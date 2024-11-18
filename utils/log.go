// Copyright (c) Jean-Francois Giorgi & AUTHORS
// parts of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause
package utils

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

// NewSLogger create a slog.Logger which displays the PC relative to a basepath
func NewSLogger(w io.Writer, level slog.Level, basePath string) *slog.Logger {

	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.SourceKey {
				s, _ := a.Value.Any().(*slog.Source)
				if s != nil {
					file, err := filepath.Rel(basePath, s.File)
					if err == nil {
						s.File = file
					}
				}
			}
			return a
		},
	}))
}

// call this to se base path relat
func ThisBasePath() string {
	_, file, _, ok := runtime.Caller(1)
	if ok {
		return filepath.Dir(file)
	}
	return ""
}
