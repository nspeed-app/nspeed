#! /bin/sh
# if [ $# -eq 0 ]; then
nspeed='go run cmd/nspeed/main.go'
# else
#     nspeed=$1
#     shift
# fi
#opts=$@
# echo "$nspeed" - "$opts"
test="nocrypto"

count=1
test $1 && {
  count=$1
  echo using count of $count
}

case $test in
	nocrypto)
        $nspeed $opts server -n $count get -n $count -w 1 localhost:7333/20g
        $nspeed $opts server -h2c -n $count get -n $count -h2c -w 1 localhost:7333/20g
        break
        ;;
	crypto)
        $nspeed $opts server -self -n $count get -n $count -self -http11 -w 1 https://localhost:7333/20g
        $nspeed $opts server -self -n $count get -n $count -self -w 1 https://localhost:7333/20g
        # http/3
        $nspeed $opts server -http3 -n $count get -n $count -self -w 1 -http3 https://localhost:7333/20g
		break
        ;;
	*)
		echo "unknown case"
        ;;
  esac
