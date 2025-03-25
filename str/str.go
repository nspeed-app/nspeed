// Copyright (c) Jean-Francois Giorgi & AUTHORS
// part of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause

package str

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"runtime"
	"strings"

	"github.com/rs/zerolog/log"
)

// StringArray is just a []string. Used for parsing multiple ocurrences of the same string flag (see flag.Var)
type StringArray []string

func (s *StringArray) String() string {
	return fmt.Sprintf("%+v", *s)
}
func (s *StringArray) Set(value string) error {
	*s = append(*s, value)
	return nil
}

// Drop-in replacement for os.File but interprets "-" as stdout
// ignore Close when stdout
type SmartFile struct {
	*os.File
	isstd bool
}

func (f *SmartFile) Close() error {
	if f.isstd {
		return nil
	}
	if f.File == nil {
		return os.ErrInvalid
	}
	err := f.File.Close()
	f.File = nil
	return err
}

// FileCreate is like os.Create() but treats filename "-" as stdout
func FileCreate(filename string) (*SmartFile, error) {
	if filename == "-" {
		return &SmartFile{os.Stdout, true}, nil
	}
	fd, err := os.Create(filename)
	if err != nil {
		return nil, err
	}
	return &SmartFile{fd, false}, nil
}

// DumpRequest is a shortcut to httputil.DumpRequestOut
func DumpRequest(req *http.Request, body bool) {
	dump, err := httputil.DumpRequestOut(req, body)
	if err != nil {
		log.Fatal().Err(err).Msg("DumpRequestOut failed")
	}

	fmt.Printf("%s", dump)
}

// ToArgv converts string s into an argv for exec.
// copied from : https://github.com/mgutz/str (MIT License)
func ToArgv(s string) ([]string, error) {
	const (
		InArg = iota
		InArgQuote
		OutOfArg
	)
	currentState := OutOfArg
	currentQuoteChar := "\x00" // to distinguish between ' and " quotations
	// this allows to use "foo'bar"
	currentArg := ""
	argv := []string{}

	isQuote := func(c string) bool {
		return c == `"` || c == `'`
	}

	isEscape := func(c string) bool {
		return c == `\`
	}

	isWhitespace := func(c string) bool {
		return c == " " || c == "\t"
	}

	L := len(s)
	for i := 0; i < L; i++ {
		c := s[i : i+1]

		//fmt.Printf("c %s state %v arg %s argv %v i %d\n", c, currentState, currentArg, args, i)
		if isQuote(c) {
			switch currentState {
			case OutOfArg:
				currentArg = ""
				fallthrough
			case InArg:
				currentState = InArgQuote
				currentQuoteChar = c

			case InArgQuote:
				if c == currentQuoteChar {
					currentState = InArg
				} else {
					currentArg += c
				}
			}

		} else if isWhitespace(c) {
			switch currentState {
			case InArg:
				argv = append(argv, currentArg)
				currentState = OutOfArg
			case InArgQuote:
				currentArg += c
			case OutOfArg:
				// nothing
			}

		} else if isEscape(c) {
			switch currentState {
			case OutOfArg:
				currentArg = ""
				currentState = InArg
				fallthrough
			case InArg:
				fallthrough
			case InArgQuote:
				if i == L-1 {
					if runtime.GOOS == "windows" {
						// just add \ to end for windows
						currentArg += c
					} else {
						return nil, fmt.Errorf("escape character at end string")
					}
				} else {
					if runtime.GOOS == "windows" {
						peek := s[i+1 : i+2]
						if peek != `"` {
							currentArg += c
						}
					} else {
						i++
						c = s[i : i+1]
						currentArg += c
					}
				}
			}
		} else {
			switch currentState {
			case InArg, InArgQuote:
				currentArg += c

			case OutOfArg:
				currentArg = ""
				currentArg += c
				currentState = InArg
			}
		}
	}

	if currentState == InArg {
		argv = append(argv, currentArg)
	} else if currentState == InArgQuote {
		return nil, fmt.Errorf("starting quote has no ending quote")
	}

	return argv, nil
}

// Fields is like string.Fields but dont split double-quoted strings
// if parsing fails, nil is returned
func Fields(s string) []string {
	r := csv.NewReader(strings.NewReader(s))
	r.Comma = ' '
	r.TrimLeadingSpace = true
	fields, err := r.Read()
	if err != nil {
		return nil
	}
	return fields
}
