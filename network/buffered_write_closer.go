// Copyright (c) Jean-Francois Giorgi & AUTHORS
// part of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause

package network

import (
	"bufio"
	"errors"
	"io"
)

type bufferedWriteCloser struct {
	*bufio.Writer
	io.Closer
}

// NewBufferedWriteCloser creates an io.WriteCloser from a bufio.Writer and an io.Closer
func NewBufferedWriteCloser(writer *bufio.Writer, closer io.Closer) io.WriteCloser {
	return &bufferedWriteCloser{
		Writer: writer,
		Closer: closer,
	}
}

func (h bufferedWriteCloser) Close() error {
	if h.Writer == nil {
		return errors.New("bufferedWriteCloser is missing a bufio.Writer")
	}
	if err := h.Writer.Flush(); err != nil {
		return err
	}
	return h.Closer.Close()
}
