#git clone https://cs.opensource.google/go/x/net ../golang-net
git -C ../../golang-net fetch origin
git -C ../../golang-net reset --hard origin/master
git -C ../../golang-net fetch https://go.googlesource.com/net refs/changes/34/362834/12
git -C ../../golang-net cherry-pick FETCH_HEAD
git -C ../../golang-net fetch https://go.googlesource.com/net refs/changes/97/150197/5 
git -C ../../golang-net cherry-pick FETCH_HEAD