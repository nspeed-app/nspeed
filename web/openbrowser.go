// Copyright (c) Jean-Francois Giorgi & AUTHORS
// part of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause
package web

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
// could import this pkg but it has a stdout/stdin issue
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
// This is non blocking and will return immediately once the browser is launched
func OpenBrowser(url string, stdout io.Writer, stderr io.Writer) error {
	var err error
	switch runtime.GOOS {
	case "linux":
		providers := []string{"open", "xdg-open", "x-www-browser", "www-browser"}

		// There are multiple possible providers to open a browser on linux
		// Usually "open" is enough but sometimes other commands work too.
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
// To allow full HTML/JS/CSS content to work, the html content is not opened as a local file but
// served using a one-time use web server on localhost (random port).
// if openBrowser is false the server url will be printed
func OpenHTML(ctx context.Context, html string, openBrowser bool) error {
	server := &http.Server{}

	// cancel() will allow to shutdown the server
	// ctx will allow to wait for cancel() to be called
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// serve html content and then cancel() the server
	sendHTML := func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, html)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		cancel()
	}
	http.HandleFunc("/", sendHTML)
	server.Handler = http.DefaultServeMux

	ln, err := net.Listen("tcp", "localhost:0")
	defer func() {
		_ = ln.Close()
	}()
	if err != nil {
		return err
	}

	if openBrowser {
		err = OpenBrowser("http://"+ln.Addr().String(), nil, nil)
		if err != nil {
			return err
		}
	} else {
		fmt.Println("page url = http://" + ln.Addr().String())
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
