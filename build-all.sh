#! /bin/bash
GOOS=linux GOARCH=amd64 go build -o distrib/nspeed_linux_amd64 -ldflags "-X main.Version=$(git describe --tags)" cmd/nspeed/main.go
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o distrib/nspeed_windows_amd64.exe -ldflags "-X main.Version=$(git describe --tags)" cmd/nspeed/main.go
#GOOS=darwin GOARCH=amd64 go build -o distrib/nspeed_darwin_amd64 -ldflags "-X main.Version=$(git describe --tags)" 
sh build-darwin.sh