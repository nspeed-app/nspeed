# v0.8
## general
- a few typos fixes
- added `-h2c` mode to client & server to allow HTTP/2 Cleartext (H2C)
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
  - `ct` query parameter added: set the returned content-type header, for instance: http://localhost:7333/1k.jpg?ct=text/plain will return a content-type of `text/plain` instead of `image/jpeg` ("ct" has precedence oever the extension. With no precedence or `ct` parameter, the default content-type is `application/octet-stream` ).
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
 - "-d duration" duration after which the server shutdown (duration must have a unit: s,m or h and a combinaison : 5h20m for instance)
 - "-n value" number of requests after which  the server shutdown
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
  - put: redirect(s) happen after the upload which normal behavior for a HTTP POST.
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
 
