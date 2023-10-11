# run all bench
for i in benchs/*; do go run cmd/nspeed/main.go-json test/$(basename $i).json from $i; done