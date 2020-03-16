#! /bin/bash
url=$1
echo url = $url
fqdn="$(echo $url | awk -F/ '{ print $3}')"
echo fqdn = $fqdn
ip="$(getent hosts $fqdn | awk '{ print $1 }')"
echo ip = $ip
ifname="$(ip route get $ip | sed -n 's/.*dev \([^\ ]*\) .*/\1/p')"
echo interface = $ifname
echo starting curl
start_time=`date +%s%N`
before=$(cat /sys/class/net/$ifname/statistics/rx_bytes)
curl -o /dev/null -s -w "@curl-format.txt" $url
after=$(cat /sys/class/net/$ifname/statistics/rx_bytes)
end_time=`date +%s%N`
echo curl ended
traffic=$(($after-$before))
dur=$((($end_time-$start_time)/1000000))
echo $traffic bytes on interface $ifname took $dur msecs = $(($traffic*8/$dur/1000)) Mbps
