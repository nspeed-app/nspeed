# NSpeed
A client and server high performance network bandwidth measurement tool using Internet standards (HTTP/1, HTTP/2, HTTP/3*, WebRTC, WebTransport*)
Interoperable with standard web clients (like curl) and standard web servers (NGinx, Apache, etc)

## Usage
    nspeed [global options] [command] ... [command]

    available commands:
    - get
    - put
    - server
    - ciphers
    - api

    The ciphers command can not be combined with others and can only be called once. 
    Each "command" has its own arguments, use -h to see them. 
    
    Overview of commands:

        nspeed get [options] url 
        nspeed put [options] url size
        nspeed server [options]
        nspeed ciphers [options] target
        nspeed api [options]

## Examples

    # download a single target four times at the same time
    nspeed get -n 4 https://bouygues.testdebit.info/10G/10G.iso

    # download 2 different targets
    nspeed get https://bouygues.testdebit.info/10G/10G.iso get https://scaleway.testdebit.info/10G/10G.iso

    # downlaod the same target both in IPv4 and IPv6
    nspeed get -4 https://bouygues.testdebit.info/100M/100M.iso get -6 https://bouygues.testdebit.info/100M/100M.iso

    # upload two 1GB to a single target (use "1g" for 1GB (1000*1000*1000 bytes) and "1G" for 1GiB (1024*1024*1024 bytes)
    # accepted prefixes: k,m,g,t,p,e and K,M,G,T,P,E
    nspeed put -n 2 https://bouygues.testdebit.info/ 1g

    # download & upload at the same time
    nspeed put https://bouygues.testdebit.info/ 1g get https://bouygues.testdebit.info/1G/1G.iso

    # start a server with default settings (port: 7333, max time 10s , max size 1.1 TB)
    nspeed server
    
    # start a server at port 8888 with max time of 5 seconds and 1 GB max size accepted
    nspeed server -p 8080 -t 5 -s 1000000000

    # start a server listening on all interfaces but in IPv6 only
    nspeed server -6 -a=""

    # start a server listening on a specific IPv4 address
    nspeed server -a 192.168.1.3

    # start two instances of server, one listening on a specific IPv6 address and one on a specific IPv4 address
    nspeed server -a 2001:1234:5678::3 server -a 192.168.1.3

    # download 1GB from a nspeed server
    nspeed get http://localhost:7333/1g

    # download 20 x 100MiB from a nspeed server
    nspeed get -n 20 http://localhost:7333/100M

    # download 1GiB from a nspeed local server with curl
    curl -o /dev/null http://localhost:7333/1G

    # upload a local file "/path/to/file" to a nspeed server with curl (with progress and result speed)
    curl -T /path/to/file http://localhost:7333/ | tee

    # same as above but without sending the filename to the server
    curl -X POST --data-binary /path/to/file http://localhost:7333/ | tee

    # download 1 GB from the local server and stop it
    # how it works:
    #   - we launch a localhost server that will respond to one request then shutdown (-n 1)
    #   - we launch a get command that will wait 1 second (-w 1) before requesting the server
    nspeed server -n 1 get -w 1 http://localhost:7333/1g

    #download 1000 bytes as content-type "text/plain" but with extention "jpg" from a nspeed server with curl
    curl -o /dev/null http://localhost:7333/1k.jpg?ct=text/plain

## Installation

Binary distribution available here: [dl.nspeed.app](https://dl.nspeed.app) or in the [release section](https://github.com/nspeed-app/nspeed/releases)

Download the one for your system and eventually rename it to `nspeed`.
On Unix systems make the file executable with: `chmod +x nspeed` 

Source code with be released before v1.0

## What's next?
- [ ] web UI (local & remote)
- [ ] remote agent
- [ ] P2P (WebRTC)
- [ ] QUIC & HTTP/3 (waiting on Go code)
- [ ] formatted metrics (might be OpenMetrics ?)
- [ ] network & hardware information (routes, pci bandwidth, NIC info, erros, etc)
- [ ] gateway/router info & crosstalk information if available 

## Acknowledgement
- Vivien Guéant & everyone at [lafibre.info](https://lafibre.info) for spawning the ideas and their feedback and testing.
- Vivien Guéant & [L'ARCEP][arcep] for the ['2020 Open Internet' publication][rapport]
- Artyom Pervukhin ( https://github.com/artyom ) for hints
- Will McCutchen ( https://github.com/mccutchen ) for go-httpbin
- Francesc Campoy for the [JustForFunc series](https://www.youtube.com/c/JustForFunc/videos)
- Joe Shaw for [Abusing go:linkname to customize TLS 1.3 cipher suites](https://www.joeshaw.org/abusing-go-linkname-to-customize-tls13-cipher-suites/)

[arcep]: https://arcep.fr/
[rapport]: https://en.arcep.fr/news/press-releases/view/n/internet-ouvert.html

## Support or Contact

[info@nspeed.app](mailto:info@nspeed.app)
