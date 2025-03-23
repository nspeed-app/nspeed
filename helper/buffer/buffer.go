package buffer

import (
	"fmt"
	"io"
	"math/rand"
	"time"
)

// BigBuffer is used with io.CopyBuffer because of io.Copy internal buffer is too small.
// todo: we set a 4 MiB size. on small systems/embedded systems this could be an issue
// This buffer can be read/written by multiple goroutines at the same time so do not use the data in it.
// This will generate a data race
// io.CopyBuffer default size is 32 * 1024
const maxBufferSize = 4 * 1024 * 1024

var bigBuffer [maxBufferSize]byte
var buffer = bigBuffer[:]
var useBuffer = false

// UseBuffer set the global buffer size for i/o copy. if negative use max size.
// return the size
func UseBuffer(size int64) (int64, error) {
	if size > maxBufferSize {
		return 0, fmt.Errorf("buffer size is too big, max is %d", maxBufferSize)
	}
	if size < 0 {
		size = maxBufferSize
	}
	useBuffer = true
	buffer = bigBuffer[:size]
	return size, nil
}

// BigChunk is a big array of random data
const MaxChunkSize = 4 * 1024 * 1024 // warning : this will be allocated in memory
var bigChunk [MaxChunkSize]byte

// GetChunk return the address of a buffer of given size. It's truncated to MaxChunkSize.
func GetChunk(size int64) []byte {
	return bigChunk[:min(size, MaxChunkSize)]
}

// init a buffer of random values - call only once
func initBigChunk(seed int64) {
	rng := rand.New(rand.NewSource(seed))
	for i := int64(0); i < MaxChunkSize; i++ {
		bigChunk[i] = byte(rng.Intn(256))
	}
}

func init() {
	initBigChunk(time.Now().Unix())
}

// Copy is like io.Copy or io.CopyBuffer depending on a setting
func Copy(dst io.Writer, src io.Reader) (written int64, err error) {
	if useBuffer {
		return io.CopyBuffer(dst, src, buffer)
	}
	return io.Copy(dst, src)
}
