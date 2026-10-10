//go:build mage

package main

import (
	"github.com/magefile/mage/sh"
)

var Default = Run

const (
	uiRoot = "nspeed/embed/ui/app.wasm"
	uiMain = "nspeed/ui/goapp/main/main.go"
	Main   = "cmd/nspeed/main.go"
	Args   = "ui"
)

func Run() error {
	err := sh.RunWithV(map[string]string{"GOOS": "js", "GOARCH": "wasm"}, "go", "build", "-o", uiRoot, uiMain)
	if err != nil {
		return err
	}
	return sh.RunV("go", "run", Main, Args)
}
