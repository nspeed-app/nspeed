# darwin 
docker run -it --rm \
-v $(pwd):/go/src/nspeed \
-w /go/src/nspeed \
neilotoole/xcgo:latest goreleaser build -f .goreleaser-darwin.yml --snapshot --rm-dist
