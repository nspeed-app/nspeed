# v0.0.11
- **new command**: `from uri` (abbr. `f`) will read commands from the file/url (use `-` for stdin)
- **new command**: `then [options]` (abbr. `t`) execute previous command(s) before continuing.
- **new command**: `bench` (abbr `b`) (see corresponding bench section below)
- using semantic version (before it would have been **v0.11** -> now it's **v0.0.11**)
- using goreleaser
- using GOAMD64 levels: (see https://github.com/golang/go/wiki/MinimumRequirements#amd64 ). for now only level1 (_v1) is built.
- less info displayed with `-version`. Use `-version full` for more infos.
- **breaking changes in behavior**: 'infinite' commands (commands that usually never end like `server` and `api`) are stopped if combined with "short-lived" commands like `get` or `put` (for instance `nspeed server get http://google.com/` will end after the `get` is finished. In previous version it would wait forever for a kill signal). If all commands are "infinite" then the program will not stop until killed (=daemon mode)
- cleaned out text output, reformatted cpu output. Verbose & debug messages now display the job name at the beginning of the line. 
- **new flags**: `-display-log-level` and `-display-log-time` to toggle their respective fields.
- **breaking change**: the `-tick duration` flag is now a duration instead of an integer of seconds (default remain 1 second). It doesn't control the sampling rate of `-cpu` anymore (see `-cpurate`). The `-tick` rate is the rate of trace & debug messages.
- **new flag**: `-rate duration` used to set the frequency generating the time series data for json/html output Default is 1s.
- **new flag**: `-cpurate duration` used to set the cpu sampling rate (implies `-cpu`)
- **new flag**: `-html filename` to record the results to an interactive html file (implies `-pre 1s` and `-post 1s` flags unless set explicitly to something else). Use `-rate` to add time series.
- **new flag**: `-json filename` to record the results to a json file. Use `-rate` to add time series.
- **new flag**: `-dns-server address[:port]` to specify a dns server (literal ip address with optional port)
- **new flag**: `-ungroup` show each stream result of a multi-streams command (see client below) 
## bench
- new `bench` command (abbrev `b`) : allow to launch loopback tests (get/put on a localhost server). see `nspeed bench -h` for details.
## server
- *breaking changes*: `-t` flag renamed to `-m` to be on par with Curl.
 - added `-http1.1` and `-http2` flags to enforce the HTTP version used
 - *breaking change*: the default port is now a *random , available port instead of 7333*. Use `-p` to specific a port.
 - added `-id string` flag: allows to set a uniq id/name for this server (can be used in `nspeed:` url scheme)
 - An upload to the server (`PUT` and `POST`) now returns metrics data in json format
 - A download from a server returns a special header named `x-nspeed-guid` which value is a short-lived unique identifier of this download Sending a request to the server  with the same header will return a the metrics data in json of this download. The delay is 5 seconds.
## Client
- *breaking changes*: `-http11` flag renamed to `-http1.1`
- *breaking changes*: `-t` flag renamed to `-m` to be on par with Curl.
- in conjunction with the new server `-id` flag, a special new url scheme `nspeed:name` is now supported to reference the url address of the corresponding server (draft)
- added flag: `-connect-timeout` maximum duration for the initial connection (=timeout before dial abort)
- added `-http2` flag to force using HTTP/2
- added `-id string` flag: allows to set a uniq id/name for this command
- repeated commands (`-n` flag) are now summed and grouped in the report. use the new global flag `-ungroup` to see each instance.
## ciphers
- fixed TLS version text messages
# v0.10
## general
- experimental support of HTTP/3 using quic-go ( see HTTP/3 section )
- internal optimization to buffer sizes and http/2 issues (tracking various Go HTTP/2 performance issue being addressed by the Go team)
- using Go 1.19. `nspeed -version` now also displays the Go version used to build the binary as well as OS/Arch information
- new `-pre duration` option: wait `duration` before starting command(s) (`duration` uses Go syntax: "2s" for 2 seconds for instance)
- new `-post duration` option: wait `duration` after all commands have ended
- new `-info` flag to display some os/hardware information
- new `-text filename` flag: report the results to a text file (use `-` for stdout which is the default)
- new `-trace` flag: display lot of debug/trace information (wip - mainly used for quic/http3 tracing)
- switch to psutil v3
- fix global timeout
## API
- new endpoints: `/stats/info` and `/stats/mem` , see [API.md](API.md)
- new flags: `-stats`, `-statsonly` and `-statsdebug`: enable real time web UI stats view at `/rl` url path. `statsonly` disable all other api routes. `statsdebug` adds Go runtime specifics stats at url path: `/debug/statsviz`
- new flag: `-browse` : open the default web browser at `/rl` url

## server
 - new option `-http3` to enable HTTP/3. Implies `-self` if no TLS cert & key files are provided.

## Client
- **breaking change**: HTTP client can now do `PUT` or `POST` with corresponding matching command names. A new `post` command was added to do the POST HTTP method and the previously `put` command does now a 'PUT' method instead of a POST
- new option `-http3` to enable HTTP/3 (this will force HTTP/3 if the server support it and fallback to HTTP/2 or HTTP/1.1 if not). Implies `-self` if no TLS cert & key files are provided.
- console output displays news information per job: final target IP, latency and protocol
- default to `https://` if no scheme provided (`nspeed get google.com` is the same as `nspeed get https://google.com`)

## ciphers
- updated for Go 1.17+ internal changes
- **this command can no longer be called with other commands even with itself because of a global side-effect/trick**. This command is not available thru the API.
- displays AES hardware support

## HTTP/3
The implementation used is https://github.com/quic-go/quic-go. In trace mode (`-trace`) QLog trace files are generated in the current directory. They can be analyzed with https://qvis.quictools.info/

# v0.9
## general
- lots of internal refactoring (for the api/ui and a new internal scheduling/cancellation system)
- command names can be shortened as long as it is not ambiguous. For instance `ge` or just `g` can be used instead of `get`.
- added `l[atency]` and `t[raceroute]` command names (*placeholder, these commands are not yet implemented*)
- added `api` command (see below and [API.md](API.md))
## api
 - a new `api` command allowing NSpeed to be controlled by a simple HTTP request (REST). The `api` command create a new API endpoint with default value (localhost,7333). The `server` command can also be an API endpoint with the new `-api` flag. See [API.md](API.md) for more information about the API.
 - for now the `api` only return text strings. Later the a standard metrics format will be used.
## client
 - `get` and `put` mandatory arguments (`-url` and `-url` and `-size` respectively) can now be prefixed with a flag keyword allowing to change their order. for instance `nspeed get -4 http://google.com` is equivalent to `nspeed get -url http://google.com -4`
 - IP version displayed in text results
## server
- `api` flag: see the `api` command
- `-dir path`,serve static files from path. path is a local directory to serve content from (from `/dir` url ). The max duration parameter applies (`-t`) but not the max size parameter (`-s`).
## ciphers
- "cypher" spelling replaced with "cipher" everywhere including the `ciphers` command ("cypher" is a minority spelling so less prone to be known or searched for)
# v0.8
## general
- a few typos fixes
- added `-h2c` mode to client & server to allow HTTP/2 Cleartext (H2C)
## server
 - 
## client
 - `-http11` flag to force HTTP 1.1 when connecting to HTTP/2 server
## cyphers
 - new command `cyphers` to list supported cyphers with that server (will test only TLS 1.2 and 1.3)
# v0.7
## general
- global flags:
  -  `-self`: activate self-signed certificate for all clients (get & put)
  - `-color` : use colors in output (by default there will be no color at all)
  - `-cpu` : display cpu usage (every second). `-debug` & `-verbose` don't display cpu usage anymore
- news debug metrics: `ReadCount`,`WriteCount` (how many OS level Read & Write calls were performed) and `AverageReadSize`,`AverageWriteSize` (total volume/count)
- fix some usage messages
## server
- `-self` flag: listen in https mode using a self-signed certificate
- bigger buffer when sending
- paths in url for upload (`/p`) & download (`/g`) removed. just use the root path `/` for both (see the updated README.md examples)
- download paths can now have an extension. For instance `/10G.iso`. The Content-Type header will be set accordingly to https://golang.org/pkg/mime/#TypeByExtension.
- query parameters: 
  - `ct` query parameter added: set the returned content-type header, for instance: http://localhost:7333/1k.jpg?ct=text/plain will return a content-type of `text/plain` instead of `image/jpeg` ("ct" has precedence over the extension. With no precedence or `ct` parameter, the default content-type is `application/octet-stream` ).
  - `chunk_size` query parameter limited to 1 MiB (it's allocated once, this will be tuned later)
  - `seed` query parameter removed (this will return later)
## client
- disable compression
- dns report is back in -verbose mode
- `-self` : same as global '-self' flag
## known issues / caveats
- the self-signed cert only work for 'localhost, 127.0.0.1,::1'. Next vesion will allow test https over a LAN between trusted machines, meanwhile use `-k` flag with nspeed or curl.

#v0.6
## general
 - '-a' flag now always prefers IPv6 first
 - major overhaul of displayed messages (zerolog package)
 - display usage if no argument
 - all commands can now be mixed together (*)
 - by default now nspeed outputs less messages. "-verbose" displays these messages.new option "-debug" acts like v0.5 "-verbose" option.
 - new '-log filename' flag to write result to a structured file (not finalized)
 - colorized cpu with the -verbose option
 ## server
  -"-d duration" duration after which the server shutdown (duration must have at least one unit of: s,m or h or a combination `5h20m5s` for instance)
  -"-n value" number of requests after which  the server shutdown
## client
  - "-w duration" wait delay before starting the command

 (*) The new flags allow to test with a single nspeed command, for instance:

    nspeed server -n 2 get -w 1 -n 2 http://localhost:7333/g/1g

This will launch a server and it will answer to 2 requests then stop and a "get" client that will wait 1 second and perform 2 requests to the server.

# v0.5
## general
 - better usage help text
 - better formatting in verbose mode (-verbose)
 - parsed units allow now decimal precision ("2.2g")
## server
 - "-h" host option changed to "-a" (since "-h" is standard for help)
 - default host is now "localhost"
 - "-4" and "-6" options to use IPv4 or IPv6 only (must be consistent with "-a")
 - the "-s" option can now parse units of bytes like the client (for instance: "-s 1g" for 1GB, "-s 1G" for 1GiB)
 - the "-t" now parse seconds only instead of requiring a unit (consistent with client)
 - route '/p' is also accepted instead of redirecting to '/p/'
 - added name & version in a http response header
## get & put
 - "-4" and "-6" options are now working 
 - "-k" option to ignore certificate validation
 - "-a host|interface|ip" option

## known issues / caveats
  - server: the default host is now 'localhost' which leads to bind only to IPv4 (Go bug: https://github.com/golang/go/issues/9334). A temporary workaround is to launch a second server instance with the -6 flag: `nspeed server server -6` will listen to localhost on IPv4 and IPv6. A fix will be implemented soon. Some OS can fail to resolve 'localhost' to ::1 even if they have IPv6 configured. In that case use "-a ::1" explicitly (`nspeed server server -a ::1`)
  - put/post: redirect(s) happen after the upload which normal behavior for a HTTP POST/PUT.
  - performance inconsistencies on Windows
  - '-a "" ' doesn't parse on Windows, workaround: use '-a=""' 
  - server: '-a interface_name' bind to the first candidate address (ipv6/ipv4/link-local) found for that interface. It's not the same behavior as a bind with SO_BINDTODEVICE (which is platform specific).
  - client: '-a interface_name' use the first candidate address (ipv6/ipv4/link-local) found for that interface. It doesn't perfom a source address selection depending on the destination.
 # v0.4 - 2021/04/09
 first alpha

 # v0.3 - 2020/08/18
 technical preview

 # v0.2 - 2020/08/05
 technical preview

 # v0.1 - 2020/03/21
 technical preview
 
