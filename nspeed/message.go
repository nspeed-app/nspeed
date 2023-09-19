package nspeed

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io"
)

const L4MessageCookie = "NSPEED-COOKIE-1"

const (
	L4CommandSend = iota
	L4CommandReceive
	L4CommandBye
)

type L4Message struct {
	Cookie  string
	Size    int64
	Command string
}

func NewL4Message(cmd string, size int64) L4Message {
	return L4Message{
		Cookie:  L4MessageCookie,
		Command: cmd,
		Size:    size,
	}
}

var ErrL4MessageBadCookie = errors.New("bad cookie")

func ReadL4Message(r io.Reader) (*L4Message, error) {

	if r == nil {
		return nil, fmt.Errorf("no reader")
	}

	cmdMessage := new(L4Message)
	gobobj := gob.NewDecoder(r)

	// decodes buffer and unmarshals it into a Message struct
	err := gobobj.Decode(cmdMessage)
	if err != nil {
		return nil, fmt.Errorf("bad decode message: %w", err)
	}

	if cmdMessage.Cookie != L4MessageCookie {
		return nil, ErrL4MessageBadCookie
	}
	return cmdMessage, nil
}

func WriteL4Message(w io.Writer, msg *L4Message) error {
	if msg == nil {
		return fmt.Errorf("no message")
	}
	if w == nil {
		return fmt.Errorf("no writer")
	}
	err := gob.NewEncoder(w).Encode(*msg)
	if err != nil {
		return fmt.Errorf("WriteL4Message failed: %w", err)
	}
	// try to call Flush is w supports it
	flushable, ok := w.(interface{ Flush() error })
	if ok {
		return flushable.Flush()
	}
	return nil
}
