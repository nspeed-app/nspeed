#! /bin/bash
#set +o xtrace

COMMONARGS="-buildmode=pie -trimpath"

GOOS=linux GOARCH=amd64 go build -o distrib/nspeed_linux_amd64 -ldflags "-X nspeed.app/nspeed.Version=$(git describe --tags)" $COMMONARGS cmd/nspeed/main.go
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o distrib/nspeed_windows_amd64.exe -ldflags "-X nspeed/cli.Version=$(git describe --tags)" $COMMONARGS cmd/nspeed/main.go
#GOOS=darwin GOARCH=amd64 go build -o distrib/nspeed_darwin_amd64 -ldflags "-X cli.Version=$(git describe --tags)" 
#sh build-darwin.sh