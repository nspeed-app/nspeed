package utils

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"runtime"
)

// copied from https://github.com/pkg/browser
// could import this pkg but it has as stdout/stdin issue
// a PR https://github.com/pkg/browser/pull/30 was submited
// but never approved.
// so we copied everything we need in this single file here

func runCmd(prog string, stdout io.Writer, stderr io.Writer, args ...string) error {
	cmd := exec.Command(prog, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// OpenBrowser tried to open the url with the default web browser of the system.
// This is non blocking and will spawn a browser.
func OpenBrowser(url string, stdout io.Writer, stderr io.Writer) error {
	var err error
	switch runtime.GOOS {
	case "linux":
		providers := []string{"xdg-open", "x-www-browser", "www-browser", "open"}

		// There are multiple possible providers to open a browser on linux
		// One of them is xdg-open, another is x-www-browser, then there's www-browser, etc.
		// Look for one that exists and run it
		for _, provider := range providers {
			if _, err := exec.LookPath(provider); err == nil {
				return runCmd(provider, stdout, stderr, url)
			}
		}
		err = fmt.Errorf("no web browser/method found")
	case "windows":
		return runCmd("rundll32", stdout, stderr, "url.dll,FileProtocolHandler", url)
		//err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return runCmd("open", stdout, stderr, url)
		//err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	return err
}

// OpenHTML  will try to open the html content with the default web browser of the system.
// To allow full HTML/JS/CSS to work, the html content is not opened as a local file but
// served with a one-time use web server on localhost (random port).
// if secret is not empty, the web server will stay alive until a request with secret as path
// is made.
func OpenHTML(html string) error {
	server := &http.Server{}

	// cancel() will allow to shutdown the server
	// ctx will allow to wait for cancel() to be called
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// serve html content and then cancel() the server
	getRoot := func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, html)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		cancel()
	}
	http.HandleFunc("/", getRoot)
	server.Handler = http.DefaultServeMux

	ln, err := net.Listen("tcp", "localhost:0")
	defer func() {
		_ = ln.Close()
	}()
	if err != nil {
		return err
	}

	//spawn the browser
	err = OpenBrowser("http://"+ln.Addr().String(), nil, nil)
	if err != nil {
		return err
	}

	go func() {
		// wait for cancel() to be called
		<-ctx.Done()
		// kill the server,we should eventually use a timeout here
		_ = server.Shutdown(context.Background())
	}()

	// serve and wait for error or cancel() called
	err = server.Serve(ln)
	// server.Shutdown will make server.Serve return a http.ErrServerClosed
	if err == http.ErrServerClosed {
		err = nil
	}
	return err
}
