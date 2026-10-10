/*
nspeed measures network speeds

Usage of nspeed:

	nspeed [global options] <command> ... <command>

Available commands:
  - then  : executes all previous commands then continue
  - put   : perform a transfer to measure upload speed (default HTTP PUT)
  - server: launch a nspeed server (default HTTP server)
  - ciphers: list cipher suites supported by the target server(s)
  - get   : perform a transfer to measure download speed (default HTTP GET)
  - post  : perform an HTTP POST command to measure upload speed
  - api   : enable an API endpoint
  - ui    : open a web user interface
  - bench : perform preselected commands
  - from  : read command from file (use - for stdin)

use: "nspeed <command> -h" to see each command specific arguments and options

Available global options:

	-4	use IPv4 only
	-6	use IPv6 only
	-buffer int
	  	size of I/O buffer to use (default is 0 for system size, use -1 for max)
	-cpu
	  	display cpu usage (default is every second, use -cpurate to change). Also collect mem/gc stats
	-cpuprofile string
	  	write cpu profile to file
	-cpurate duration
	  	set cpu rate (implies -cpu)
	-debug
	  	show even more progress & details
	-display-log-level
	  	display logging levels (always true if -verbose/-debug/-trace)
	-display-log-time
	  	display logging timestamps
	-dns-server string
	  	specify a dns server to use (literal ip or literal ip:port)
	-formatV4 string
	  	specific anonymizer format for IPv4 addresses (default "1")
	-formatV6 string
	  	specific anonymizer format for IPv6 addresses (default "18")
	-html string
	  	output stats in a html file (use - for stdout)
	-info
	  	display informations about the host
	-json string
	  	output stats in a json file (use - for stdout)
	-log string
	  	structured logging output filename (use - for stdout), imply -verbose
	-maxtime duration
	  	maximum default duration of each commands (default 8s)
	-memprofile string
	  	write memory profile to file
	-n string
	  	name of the first batch
	-nocolor
	  	do not use colors in text output
	-post duration
	  	duration to wait after all commands ended
	-pre duration
	  	duration to wait before starting commands
	-rate duration
	  	data sampling rate in duration (used for time series generation)
	-self
	  	use embed self-signed certificate
	-test
	  	just test arguments, do nothing
	-text string
	  	output stats as text (use - for stdout) (default "-")
	-tick duration
	  	display progress rate in duration (used for -debug, -trace and webstats (default 1s)
	-timeout uint
	  	default global timeout in seconds, 0 for no timeout
	-trace
	  	maximum debug and tracing informations
	-ungroup
	  	show members of groups
	-verbose
	  	show progress & details
	-version
	  	display version (add "all" for more build and runtime informations)
*/
package main
