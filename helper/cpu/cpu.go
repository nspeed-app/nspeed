package cpu

import "runtime"

// MaxParallelism return number of cpu cores the program can actually use
// todo: probably obsolete to do like this, just use runtime.NumCPU()
func MaxParallelism() int {
	maxProcs := runtime.GOMAXPROCS(0)
	numCPU := runtime.NumCPU()
	if maxProcs < numCPU {
		return maxProcs
	}
	return numCPU
}
