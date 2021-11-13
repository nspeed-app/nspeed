echo HTTP/1.1 without encryption
$1 server -n 1 get -w 1  http://localhost:7333/30g
$1 server -n 1 & sleep 1 ; curl -s -o /dev/null -w "%{speed_download} * 8 \n" http://localhost:7333/30g  | sed 's/,[0-9]*//' | bc | numfmt --to=si --suffix=bps

echo HTTP/1.1 with encryption
$1 server -self -n 1 get -w 1 -self -http11 https://localhost:7333/10g
$1 server -self -n 1 & sleep 1 ; curl -k --http1.1 -s -o /dev/null -w "%{speed_download} * 8 \n" https://localhost:7333/10g  | sed 's/,[0-9]*//' | bc | numfmt --to=si --suffix=bps

echo HTTP/2 with encryption
$1 server -self -n 1 get -w 1 -self  https://localhost:7333/10g
$1 server -self -n 1 & sleep 1 ; curl -k -s -o /dev/null -w "%{speed_download} * 8 \n" https://localhost:7333/10g  | sed 's/,[0-9]*//' | bc | numfmt --to=si --suffix=bps

echo HTTP/2 without encryption
$1 server -h2c -n 1 get -w 1 -h2c  http://localhost:7333/10g
$1 server -h2c -n 1 & sleep 1 ; curl --http2-prior-knowledge -s -o /dev/null -w "%{speed_download} * 8 \n" http://localhost:7333/10g  | sed 's/,[0-9]*//' | bc | numfmt --to=si --suffix=bps

