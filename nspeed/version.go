package nspeed

import "runtime/debug"

// Version can be set at link time to override debug.BuildInfo.Main.Version,
// which is "(devel)" when building from within the module. See
// golang.org/issue/29814 and golang.org/issue/29228.
var Version string = ""

var ServerVersion string = ""

func init() {
	if Version == "" {
		if buildInfo, ok := debug.ReadBuildInfo(); ok {
			Version = buildInfo.Main.Version
		} else {
			Version = "(src)"
		}
	}
	ServerVersion = Version // in case one day we have a distinct version for server
}
