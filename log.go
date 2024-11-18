// Copyright (c) Jean-Francois Giorgi & AUTHORS
// parts of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause
package nspeed

import (
	"log/slog"
	"os"

	"nspeed.app/nspeed/utils"
)

var BasePath string
var Logger *slog.Logger

// this allows to sey the base path to the directory containingg this file
func init() {
	BasePath = utils.ThisBasePath()
	Logger = utils.NewSLogger(os.Stderr, slog.LevelInfo, BasePath)
}
