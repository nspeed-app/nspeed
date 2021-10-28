# darwin 
docker run --rm --privileged \
-e CC=o64-clang -e CXX=o64-clang++ \
-v $(pwd):/go/src/nspeed \
-v /var/run/docker.sock:/var/run/docker.sock \
-w /go/src/nspeed \
neilotoole/xcgo:latest goreleaser --snapshot --rm-dist


