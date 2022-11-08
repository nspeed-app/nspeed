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

func DumpRequest(req *http.Request, body bool) {
	dump, err := httputil.DumpRequestOut(req, body)
	if err != nil {
		log.Fatal().Err(err).Msg("DumpRequestOut failed")
	}

	fmt.Printf("%s", dump)
}

func PrintMemUsage(mess string) {
	if zerolog.GlobalLevel() > zerolog.InfoLevel {
		return
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// For info on each, see: https://golang.org/pkg/runtime/#MemStats
	fmt.Printf("%s: ", mess)
	fmt.Printf("Alloc = %v B", FormatByteDecimalUnits(int64(m.Alloc)))
	fmt.Printf("\tTotalAlloc = %v B", FormatByteDecimalUnits(int64(m.TotalAlloc)))
	fmt.Printf("\tSys = %v B", FormatByteDecimalUnits(int64(m.Sys)))
	fmt.Printf("\tNumGC = %v\n", m.NumGC)
}
