package nspeed

import "runtime/debug"

// version
var Version string = ""
var ServerVersion string = ""

func init() {
	// set version: todo, move this in nspeed package
	if Version == "" {
		if buildInfo, ok := debug.ReadBuildInfo(); ok {
			Version = buildInfo.Main.Version
		} else {
			Version = "(src)"
		}
	}
	ServerVersion = Version // in case one day we have a distinct version for server
}
