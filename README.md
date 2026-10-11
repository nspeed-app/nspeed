# NSpeed
A client and server high performance network bandwidth measurement tool using Internet standards (HTTP/1, HTTP/2, HTTP/3, WebRTC*, WebTransport*)
Interoperable with standard web clients (like curl, wget and web browsers) and standard web servers (NGinx, Apache, etc)

## Installation

### Pre-built binaries

Download the archive for your system from the [GitHub releases](https://github.com/nspeed-app/nspeed/releases/latest) page and extract it. `checksums.txt` holds the SHA-256 of every file.

Releases are built for:

| OS      | Architectures                       |
|---------|-------------------------------------|
| Linux   | amd64, arm64, armv7 (`arm`), 386    |
| Windows | amd64, arm64                        |
| macOS   | amd64 (Intel), arm64 (Apple Silicon)|
| FreeBSD | amd64, arm64                        |

Linux packages are also attached to each release: `.deb` (Debian, Ubuntu), `.rpm` (Fedora, RHEL) and `.apk` (Alpine). For instance:

    sudo apt install ./nspeed_<version>_linux_amd64.deb

macOS binaries are not notarized: if Gatekeeper blocks the binary, run `xattr -d com.apple.quarantine ./nspeed`.

### With Go

Requires [Go](https://go.dev/dl/) 1.26 or newer:

    go install nspeed.app/nspeed/cmd/nspeed@latest

The binary is installed in `$(go env GOPATH)/bin` (or `$GOBIN` if set). Use `@vX.Y.Z` instead of `@latest` for a specific version.

### Build for any other target

NSpeed is pure Go, so it can be built for any target Go supports (`go tool dist list`), for instance routers or boards not covered by the releases. From a clone of this repository, set `GOOS`/`GOARCH`:

    git clone https://github.com/nspeed-app/nspeed.git && cd nspeed
    CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build -o bin/ ./cmd/nspeed

The binary is written to `bin/` (`bin/nspeed`, or `bin/nspeed.exe` for Windows). `CGO_ENABLED=0` makes it a static binary with no system library dependencies (`ldd` reports "not a dynamic executable"), so it runs on any machine of the same OS/architecture. Common values:

| Target                                 | Settings                             |
|----------------------------------------|--------------------------------------|
| MIPS big-endian (many routers)         | `GOARCH=mips GOMIPS=softfloat`       |
| MIPS little-endian (many routers)      | `GOARCH=mipsle GOMIPS=softfloat`     |
| MIPS64                                 | `GOARCH=mips64` or `GOARCH=mips64le` |
| Older ARM boards (Raspberry Pi 1/Zero) | `GOARCH=arm GOARM=6`                 |
| RISC-V                                 | `GOARCH=riscv64`                     |

## Usage
    nspeed [global options] <command> ... <command>

    Available commands:
    - get   : perform a transfer to measure download speed (receive) (default HTTP GET)
    - put   : perform a transfer to measure upload speed (transmit) (default HTTP PUT)
    - post  : perform an HTTP POST command to measure upload speed (transmit) (default HTTP POST)
    - server: launch a nspeed server (default HTTP server)
    - from  : read commands from a file or URL (use - for stdin)
    - then  : executes all previous commands then continue
    - bench : perform preselected commands
    - api   : enable an API endpoint
    - ciphers: list cipher suites supported by the target server(s)

    use: "nspeed <command> -h" to see each command specific arguments and options

    Overview of commands:

        nspeed get [options] url
        nspeed put [options] url size
        nspeed post [options] url size
        nspeed server [options]
        nspeed from [options] file|url
        nspeed then [options]
        nspeed bench [options] benchmark[,benchmark]
        nspeed api [options]
        nspeed ciphers hostname[:port]

## Examples

    # download a single target four times at the same time
    nspeed get -n 4 https://scaleway.testdebit.info/10G/10G.iso

    # download 2 different targets
    nspeed get https://scaleway.testdebit.info/10G/10G.iso get https://scaleway.testdebit.info/10G/10G.iso

    # download the same target in IPv4 and IPv6 at the same time
    nspeed get -4 https://scaleway.testdebit.info/100M/100M.iso get -6 https://scaleway.testdebit.info/100M/100M.iso

    # upload two 1GB to a single target (use "1g" for 1GB (1000*1000*1000 bytes) and "1G" for 1GiB (1024*1024*1024 bytes)
    # accepted units: k,m,g,t,p,e and K,M,G,T,P,E
    nspeed put -n 2 https://scaleway.testdebit.info/ 1g

    # download & upload at the same time
    nspeed put https://scaleway.testdebit.info/ 1g get https://scaleway.testdebit.info/1G/1G.iso

    # start a server with default settings (host: 127.0.0.1, port: random, max time 10s , max size 1.1 TB)
    nspeed server
    
    # start a server at port 8888 with max time of 5 seconds and 1 GB max size accepted
    nspeed server -p 8080 -t 5 -s 1g

    # start a server listening on all interfaces but in IPv6 only
    nspeed server -6 -a=""

    # start a server listening on a specific IPv4 address
    nspeed server -a 192.168.1.3

    # start a server listening on a specific interface using IPv4
    nspeed server -4 -a tailscale0

    # start two instances of server, one listening on a specific IPv6 address and one on a specific IPv4 address
    nspeed server -a 2001:1234:5678::3 -p 7333 server -a 192.168.1.3 -p 7333

    # download 1GB from a local nspeed server (the server generate content based on the url path: format is "/size[.ext][?ct=content-type]")
    nspeed get http://localhost:7333/1g

    # download 20 x 100MiB from a local nspeed server
    nspeed get -n 20 http://localhost:7333/100M

    # with curl, download 1GiB from a local nspeed server
    curl -o /dev/null http://localhost:7333/1G

    # with curl, upload a local file "/path/to/file" to a local nspeed server (with progress and result speed)
    curl -T /path/to/file http://localhost:7333/ | tee

    # same as above but without sending the filename to the server
    curl -X POST --data-binary /path/to/file http://localhost:7333/ | tee

    # download 1 GB from the local server and stop it
    # how it works:
    #   - we launch a localhost server with id "s1"
    #   - we launch a get command using a special scheme "nspeed://id" to get the local ip & port of "s1" server
    nspeed server -id s1 get nspeed://s1/1g

    # same using a specified port
    nspeed server -p 7333 get http://localhost:7333/1g

    # with curl, download 1k bytes as content-type "text/plain" but with extention "jpg" from a nspeed server
    curl -o /dev/null http://localhost:7333/1k.jpg?ct=text/plain

## Avanced examples (from, then, bench, api, **wip**)

    # execute a get then a put with named jobs
    nspeed get -id "Download" https://speed.cloudflare.com/__down?bytes=500000000 then post -id "Upload" https://speed.cloudflare.com/__up 200m

    # execute commands from an url
    nspeed from https://dl.nspeed.app/cf

    # execute 3 local benchmarks at the same time
    nspeed b h1g,h2g,h3g

    # execute them one after the other
    nspeed b h1g then b h2g then b h3g

    # open the monitor , wait 2 secondes and launch a local test (requires on a computer with a local web browser)
    nspeed api -browse then -pre 2s b h1g

## What's next?
- [x] web UI (local & remote) - partitally implemented
- [x] remote agent
- [ ] P2P (WebRTC)
- [x] QUIC & HTTP/3 (waiting on Go code)
- [x] formatted metrics (partial)
- [ ] network & hardware information (routes, pci bandwidth, NIC info, erros, etc)
- [ ] gateway/router info & crosstalk information if available 

## Acknowledgement
- Vivien Guéant & everyone at [lafibre.info](https://lafibre.info) for spawning the ideas and their feedback and testing.
- Vivien Guéant & [L'ARCEP][arcep] for the ['2020 Open Internet' publication][rapport]

- Artyom Pervukhin ( https://github.com/artyom ) for hints
- Will McCutchen ( https://github.com/mccutchen ) for go-httpbin
- Francesc Campoy for the [JustForFunc series](https://www.youtube.com/c/JustForFunc/videos)
- Joe Shaw for [Abusing go:linkname to customize TLS 1.3 cipher suites](https://www.joeshaw.org/abusing-go-linkname-to-customize-tls13-cipher-suites/)

- [Apache Echarts](https://echarts.apache.org/en/index.html)
- [Chart.js](https://www.chartjs.org/)

[arcep]: https://arcep.fr/
[rapport]: https://www.arcep.fr/uploads/tx_gspublication/rapport-etat-internet_edition-2020_250620.pdf

## Support or Contact

[info@nspeed.app](mailto:info@nspeed.app)
