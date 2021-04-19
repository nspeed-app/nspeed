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
  - '-a "" ' doesn't parse on Windows, workaround: use '-a ::' 
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
 