#! /bin/bash
GOOS=linux GOARCH=amd64 go build -o distrib/Linux-x64/nspeed-client client.go
GOOS=windows GOARCH=amd64 go build -o distrib/Windows-x64/nspeed-client.exe client.go
GOOS=darwin GOARCH=amd64 go build -o distrib/Mac-x64/nspeed-client client.go
