#! /bin/bash
#set +o xtrace
COMMONARGS="-buildmode=pie -trimpath"
COMMONARGS2="-trimpath"

GOOS=darwin GOARCH=amd64  go build -o distrib/nspeed_$GOOS_$GOARCH -ldflags "-X nspeed.app/nspeed.Version=$(git describe --tags)" $COMMONARGS cmd/nspeed/main.go
