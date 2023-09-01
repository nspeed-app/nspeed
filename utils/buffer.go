package utils

import (
	"math/rand"
	"time"
)

// BigBuffer is used with io.CopyBuffer because of io.Copy internal buffer is too small.
// todo: we set a 4 MiB size. on small systems/embedded systems this could be an issue
// This buffer can be read/written by multiple goroutines at the same time so do not use the data in it.
// This will generate a data race
// io.CopyBuffer default size is 32 * 1024
var bigBuffer [1024 * 1024]byte
var BigBuffer = bigBuffer[:]

// BigChunk is a big array of random data
const MaxChunkSize = 1 * 1024 * 1024 // warning : 1 MiB //this will be allocated in memory
var BigChunk [MaxChunkSize]byte

// init a buffer of random values - callable only once
// thread safe
func InitBigChunk(seed int64) {
	rng := rand.New(rand.NewSource(seed))
	for i := int64(0); i < MaxChunkSize; i++ {
		BigChunk[i] = byte(rng.Intn(256))
	}
}

// called once
func init() {
	InitBigChunk(time.Now().Unix())
}
