package nspeed

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"runtime"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
// since not YET in std lib...
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

func DumpRequest(req *http.Request, body bool) {
	dump, err := httputil.DumpRequestOut(req, body)
	if err != nil {
		log.Fatal().Err(err).Msg("DumpRequestOut failed")
	}

	fmt.Printf("%s", dump)
}

func PrintMemUsage(log *zerolog.Logger, mess string) {
	if log == nil {
		return
	}
	if log.GetLevel() > zerolog.InfoLevel {
		return
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// For info on each, see: https://golang.org/pkg/runtime/#MemStats
	log.Printf("%s: - Alloc = %vB\tTotalAlloc = %vB\tSys = %vB\tNumGC = %v\n", mess,
		FormatByteDecimalUnits(int64(m.Alloc)),
		FormatByteDecimalUnits(int64(m.TotalAlloc)),
		FormatByteDecimalUnits(int64(m.Sys)),
		m.NumGC)
}

// Partition splits an array into 2 arrays based on a predicate function
func Partition[T any](array []T, test func(T) bool) (matched []T, notmatched []T) {
	for _, t := range array {
		if test(t) {
			matched = append(matched, t)
		} else {
			notmatched = append(notmatched, t)
		}
	}
	return
}

// IndexOf searches for an element in a slice and returns its index or -1 if not found
// todo: find and use the 'canonical' name for this function
// todo: watch for future equivalence in std lib
func IndexOf[T comparable](ss []T, s T) (idx int) {
	for idx, v := range ss {
		if v == s {
			return idx
		}
	}
	return -1
}
