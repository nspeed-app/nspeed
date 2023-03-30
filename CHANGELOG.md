# v0.0.11 (draft)
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
