tests cubic vs bbr for Bytel & Online:

get latest nspeed client from: https://dl.nspeed.app/nspeed-client/preview/ :

standard computers:
  Windows : https://dl.nspeed.app/nspeed-client/preview/nspeed_windows_amd64_v1/nspeed.exe 
  Mac: https://dl.nspeed.app/nspeed-client/preview/nspeed_darwin_amd64/nspeed 
  Linux: https://dl.nspeed.app/nspeed-client/preview/nspeed_linux_amd64_v1/nspeed

On Mac/Linux: add "chmod +x ./nspeed"

check if nspeed is working:
./nspeed -rate 100ms -html : get google.com

run the tests:

./nspeed -rate 100ms -html bytel-only.html     from https://dl.nspeed.app/bctests/bytel-only
./nspeed -rate 100ms -html bytel-online.html   from https://dl.nspeed.app/bctests/bytel-online
./nspeed -rate 100ms -html bytelAndOnline.html from https://dl.nspeed.app/bctests/bytelAndOnline

# this will generate the corresponding html files, use "-json file.json" instead if you just want the results as json