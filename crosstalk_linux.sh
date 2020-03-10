#! /bin/bash
url=$1
echo url = $url
fqdn="$(echo $url | awk -F/ '{ print $3}')"
echo fqdn = $fqdn
ip="$(getent hosts $fqdn | awk '{ print $1 }')"
echo ip = $ip
interface="$(ip route get $ip | sed -n 's/.*dev \([^\ ]*\) table.*/\1/p')"
echo interface = $interface

exit
ifname="eno1"
before=$(cat /sys/class/net/$ifname/statistics/rx_bytes)
curl -o /dev/null https://ipv4.bouygues.testdebit.info/1G/1G.iso
after=$(cat /sys/class/net/$ifname/statistics/rx_bytes)
traffic=$after-$before
echo $after $before $traffic