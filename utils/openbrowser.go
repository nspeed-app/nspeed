package utils

import (
	"fmt"
	"io"
	"os/exec"
	"runtime"
)

// copied from https://github.com/pkg/browser
// could import this pkg but it has as stdout/stdin issue
// a PR https://github.com/pkg/browser/pull/30 was submited
// but never approved.
// so we copied everything we need in a single file here

func runCmd(prog string, stdout io.Writer, stderr io.Writer, args ...string) error {
	cmd := exec.Command(prog, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func Openbrowser(url string, stdout io.Writer, stderr io.Writer) error {
	var err error
	switch runtime.GOOS {
	case "linux":
		providers := []string{"xdg-open", "x-www-browser", "www-browser"}

		// There are multiple possible providers to open a browser on linux
		// One of them is xdg-open, another is x-www-browser, then there's www-browser, etc.
		// Look for one that exists and run it
		for _, provider := range providers {
			if _, err := exec.LookPath(provider); err == nil {
				return runCmd(provider, stdout, stderr, url)
			}
		}
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	return err
}
