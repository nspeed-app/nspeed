package nspeed

import "os"

// Create creates or truncates the named file.
// Drop-in replacement for os.Create but interprets "-" as stdout
func FileCreate(filename string) (*os.File, error) {
	if filename == "-" {
		return os.Stdout, nil
	}
	return os.Create(filename)
}
