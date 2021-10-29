# darwin 
docker run -it --rm \
-v $(pwd):/go/src/nspeed \
-w /go/src/nspeed \
neilotoole/xcgo:latest goreleaser release --snapshot --rm-dist
