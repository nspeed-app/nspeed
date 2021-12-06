#! /bin/sh
if [ $# -eq 0 ]; then
    nspeed='go run main.go'
else
    nspeed=$1
    shift
fi
opts=$@
# echo "$nspeed" - "$opts"
# no encryption
$nspeed $opts server -n 1 get -w 1 localhost:7333/20g
$nspeed $opts server -h2c -n 1 get -h2c -w 1 localhost:7333/20g
exit
# with encryption
$nspeed $opts server -self -n 1 get -self -http11 -w 1 https://localhost:7333/20g
$nspeed $opts server -self -n 1 get -self -w 1 https://localhost:7333/20g

