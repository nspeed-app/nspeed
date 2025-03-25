// Copyright (c) Jean-Francois Giorgi & AUTHORS
// parts of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause

package nspeed

import (
	"log/slog"
	"os"

	"nspeed.app/logging"
)

var BasePath string
var Logger *slog.Logger

// this allows to sey the base path to the directory containingg this file
func init() {
	BasePath = logging.ThisBasePath()
	Logger = logging.NewSLogger(os.Stderr, slog.LevelInfo, BasePath)
}
