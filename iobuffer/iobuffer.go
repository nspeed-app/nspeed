// Copyright (c) Jean-Francois Giorgi & AUTHORS
// part of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause

package iobuffer

import (
	"fmt"
	"io"
	"math/rand"
	"time"
)

// A buffer is used with io.CopyBuffer because of io.Copy internal buffer is too small.
// todo: we set a 4 MiB size. on small systems/embedded systems this could be an issue
// This buffer can be read/written by multiple goroutines at the same time so do not use the data in it.
// This will generate a data race
// 12/2024: io.CopyBuffer default size is 32 * 1024 (Go source: src/io/io.go copyBuffer func)

const MaxBufferSize = 4 * 1024 * 1024 // 4 MiB

var memBuffer [MaxBufferSize]byte
var iobuffer = memBuffer[:]
var useBuffer = false

// UseBuffer set the global buffer size for i/o copy. if negative use max size.
// return the size
func UseBuffer(size int64) (int64, error) {
	if size > MaxBufferSize {
		return 0, fmt.Errorf("buffer size is too big, max is %d", MaxBufferSize)
	}
	if size < 0 {
		size = MaxBufferSize
	}
	useBuffer = true
	iobuffer = memBuffer[:size]
	return size, nil
}

// Copy is like io.Copy or io.CopyBuffer depending on a setting
// investigate: for perf avoid testing useBuffer each time (use a function var ?)
func Copy(dst io.Writer, src io.Reader) (written int64, err error) {
	if useBuffer {
		return io.CopyBuffer(dst, src, iobuffer)
	}
	return io.Copy(dst, src)
}

// GetChunk return a slice of given size. It's truncated to max size of the underlying buffer.
func GetChunk(size int64) []byte {
	return memBuffer[:min(size, MaxBufferSize)]
}

// init the in memory buffer with random values
func initBuffer(seed int64) {
	rng := rand.New(rand.NewSource(seed))
	for i := int64(0); i < MaxBufferSize; i++ {
		memBuffer[i] = byte(rng.Intn(256))
	}
}
func init() {
	initBuffer(time.Now().Unix())
}
