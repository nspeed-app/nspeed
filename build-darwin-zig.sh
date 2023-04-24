#! /bin/bash
export MACOS_MIN_VER=10.14
export MACOS_SDK_PATH="$(pwd)/../MacOSX11.3.sdk"

# create dist dir

# build
export CGO_ENABLED="1"
export GOOS="darwin"
export GOARCH="amd64"
export CGO_LDFLAGS="--sysroot ${MACOS_SDK_PATH} -F/System/Library/Frameworks -L/usr/lib"
export CC="zig cc -target x86_64-macos -isysroot ${MACOS_SDK_PATH} -iwithsysroot /usr/include -iframeworkwithsysroot /System/Library/Frameworks" 
export CXX="zig c++ -target x86_64-macos -isysroot ${MACOS_SDK_PATH} -iwithsysroot /usr/include -iframeworkwithsysroot /System/Library/Frameworks"
go build -v -ldflags="-s -w -X nspeed.app/nspeed.Version=$(git describe --tags)" -trimpath -buildmode=pie -o dist/nspeed_${GOOS}_${GOARCH}/nspeed  ./cmd/nspeed/main.go
# ldflafs -s -w is to prevent error , see https://zig.news/kristoff/building-sqlite-with-cgo-for-every-os-4cic
#echo $GOOS \\n $GOARCH \\n $CGO_LDFLAGS \\n $CC \\n $CXX
