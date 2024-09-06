package nspeed

import (
	"errors"
	"fmt"
)

type NSError struct {
	Value string
	Err   error
}

func NewNSError(err error, value string) *NSError {
	return &NSError{
		Value: value,
		Err:   err,
	}
}

func (e *NSError) Error() string {
	return fmt.Sprintf("nspeed error: %s (%s)", e.Err, e.Value)
}

func (e *NSError) Unwrap() error {
	return e.Err
}

var ErrNotImplemented = errors.New("not implemented yet")
var ErrNotSupported = errors.New("not supported")
