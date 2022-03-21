package nspeed

import (
	"os"

	"golang.org/x/exp/constraints"
)

// Create creates or truncates the named file.
// Drop-in replacement for os.Create but interprets "-" as stdout
func FileCreate(filename string) (*os.File, error) {
	if filename == "-" {
		return os.Stdout, nil
	}
	return os.Create(filename)
}

// min/max with generics
// not in std lib...
func Min[T constraints.Ordered](x, y T) T {
	if x < y {
		return x
	}
	return y
}

func Max[T constraints.Ordered](x, y T) T {
	if x > y {
		return x
	}
	return y
}
