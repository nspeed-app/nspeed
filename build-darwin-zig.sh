# doesn't work yet
export MACOS_MIN_VER=10.14
export MACOS_SDK_PATH="$(pwd)/../MacOSX11.3.sdk"

# create dist dir
mkdir -p dist/darwin-amd64

# build
CGO_ENABLED=1 \
GOOS=darwin \
GOARCH=amd64 \
CGO_LDFLAGS="-mmacosx-version-min=${MACOS_MIN_VER} --sysroot ${MACOS_SDK_PATH} -F/System/Library/Frameworks -L/usr/lib" \
CC="zig cc -mmacosx-version-min=${MACOS_MIN_VER} -target x86_64-macos-gnu -isysroot ${MACOS_SDK_PATH} -iwithsysroot /usr/include -iframeworkwithsysroot /System/Library/Frameworks" \
CXX="zig c++ -mmacosx-version-min=${MACOS_MIN_VER} -target x86_64-macos-gnu -isysroot ${MACOS_SDK_PATH} -iwithsysroot /usr/include -iframeworkwithsysroot /System/Library/Frameworks" \
# ldflafs -s -w is to prevent error , see https://zig.news/kristoff/building-sqlite-with-cgo-for-every-os-4cic
go build -ldflags="-s -w" -trimpath -buildmode=pie -o dist/darwin-amd64  ./cmd/nspeed/main.go
