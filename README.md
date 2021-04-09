# NSpeed
A client and server high performance network bandwidth measurement using Internet standards (HTTP/1, HTTP/2, HTTP3, WebRTC)
Interoperaable with standard clients and servers

## Usage
    nspeed [global options] [command] ... [command]

    available commands:
    - get
    - put
    - server

    The "server" command can only be combined with itself. others commands can be mixed.
    each "command" has its own arguments, use -h to see them. `nspeed get -h` for instance

    nspeed get [options] url 

    nspeed put [options] url size

    nspeed server [options]

    as of v0.4 the "-4" and "-6" options are not yet implemented

## Examples

    # download four times a single target
    nspeed get -n 4 https://bouygues.testdebit.info/10G/10G.iso

    # download 2 different targets
    nspeed get https://bouygues.testdebit.info/10G/10G.iso https://scaleway.testdebit.info/10G/10G.iso

    # upload 2 1GB to a single target (use "1G" for 1GiB -accepted prefixes: k,m,g,t,p,e)
    nspeed put -n 2 https://bouygues.testdebit.info/ 1g

    # download & upload at the same time
    nspeed put https://bouygues.testdebit.info/ 1g get https://bouygues.testdebit.info/1G/1G.iso

    # start a server with default settings (port: 7333, max time 10s , max size 1.1 TB)
    nspeed server
    
    # start a server at port 8888 with max time of 5 seconds and 1 GB max load
    server -p 8080 -t 5s -s 1000000000

    # download 1GB from a nspeed server
    nspeed get http://localhost:7333/g/1g

    # download 20 x 100MiB from a nspeed server
    nspeed get -n 20 http://localhost:7333/g/100M


## Installation

Binary distribution available here: [dl.nspeed.app](https://dl.nspeed.app) or in the [release section](https://github.com/nspeed-app/nspeed/releases)

These binaries are for Windows, Linux and Darwin (MacOs).
Download the one for your system and rename it to nspeed.
On Unix systems: chmod +x nspeed

Source code with be released with v1.0

## Development

"commands" with planned implementation:
  - ping (icmp/udp/tcp latency)
  - p2p (webrtc over udp with STUN/ICE nat traversal)

wip

## Acknowledgement
- Vivien Guéant & everyone at [lafibre.info](https://lafibre.info) for spawning the ideas and their feedback and testing.
- Vivien Guéant & [L'ARCEP][arcep] for the ['2020 Open Internet' publication][rapport]
- Artyom Pervukhin ( https://github.com/artyom ) for hints
- Will McCutchen ( https://github.com/mccutchen ) for go-httpbin
- Francesc Campoy for the [JustForFunc series](https://www.youtube.com/c/JustForFunc/videos)

[arcep]: https://arcep.fr/
[rapport]: https://en.arcep.fr/news/press-releases/view/n/internet-ouvert.html

## Support or Contact

[info@nspeed.app](mailto:info@nspeed.app)
