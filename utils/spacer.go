package utils

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

// Spacer is an io.Writer that add a given message if a given delay is elasped between 2 calls to Write
// This is usefull to add a visual info in real time console log for instance
// message can contain %d to display the elasped duration
type Spacer struct {
	io.Writer
	discard  bool
	delay    time.Duration
	last     time.Time
	notFirst bool
	message  func(time.Duration) string
	mu       sync.Mutex
}

func SpacerNewLine(_ time.Duration) string {
	return "\n"
}

func SpacerNewLineDelay(d time.Duration) string {
	return fmt.Sprintf("%s\n", d)
}

// NewSpacer creates a new Spacer
func NewSpacer(w io.Writer, delay time.Duration, discard bool, msgf func(time.Duration) string) *Spacer {
	return &Spacer{
		Writer:  w,
		discard: discard,
		delay:   delay,
		message: msgf,
	}
}

func (s *Spacer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	elapsed := time.Since(s.last)
	s.last = time.Now()

	if s.notFirst && (elapsed >= s.delay) {
		n, err := s.Writer.Write([]byte(s.message(elapsed)))
		if err != nil {
			s.notFirst = true
			return n, err
		}
	}
	s.notFirst = true
	if s.discard {
		return len(p), nil
	}
	return s.Writer.Write(p)
}

func (s *Spacer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.notFirst = false
	s.last = time.Now()
	if c, ok := s.Writer.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// TermFileGetSize return the terminal size is f is a tty or 0,0 if not
func TermFileGetSize(f *os.File) (width int, height int) {
	fd := int(f.Fd())
	if term.IsTerminal(fd) { //todo: do we need to check if term ?
		width, height, _ = term.GetSize(fd)
	}
	return
}
