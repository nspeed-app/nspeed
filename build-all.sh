#! /bin/bash
GOOS=linux GOARCH=amd64 go build -o distrib/nspeed_linux_amd64 client.go
GOOS=windows GOARCH=amd64 go build -o distrib/nspeed_windows_amd64.exe client.go
GOOS=darwin GOARCH=amd64 go build -o distrib/nspeed_darwin_amd64 client.go
