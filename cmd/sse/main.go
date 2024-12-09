package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"nspeed.app/utils"

	_ "embed"
)

//go:embed *.html
var rootFS embed.FS

func main() {
	nobrowser := flag.Bool("nb", false, "do not open browser")

	flag.Parse()
	fs := http.FileServerFS(rootFS)
	http.Handle("/", fs)

	files, _ := rootFS.ReadDir("/")
	for _, file := range files {
		fmt.Println(file.Name(), file.IsDir())
	}
	fmt.Println("done")
	http.HandleFunc("/events", sseHandler)

	addr := "localhost:8080"
	l, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}

	if !*nobrowser {
		utils.OpenBrowser("http://"+addr, nil, nil)
	}
	if err := http.Serve(l, nil); err != nil && err != http.ErrServerClosed {
		log.Fatalf("unable to start server: %s", err.Error())
	}
}

func sseHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	w.Header().Set("Access-Control-Allow-Origin", "*")

	memT := time.NewTicker(time.Second)
	defer memT.Stop()

	cpuT := time.NewTicker(time.Second)
	defer cpuT.Stop()

	clientGone := r.Context().Done()

	rc := http.NewResponseController(w)
	fmt.Println("SSE loop for", r.RemoteAddr)
clientalive:
	for {
		select {
		case <-clientGone:
			fmt.Println("client has disconnected")
			break clientalive
		case <-memT.C:
			m, err := mem.VirtualMemory()
			if err != nil {
				log.Printf("unable to get mem: %s", err.Error())
				return
			}

			if _, err := fmt.Fprintf(w, "event:mem\ndata:Total: %d, Used: %d, Perc: %.2f%%\n\n", m.Total, m.Used, m.UsedPercent); err != nil {
				log.Printf("unable to write: %s", err.Error())
				return
			}

			rc.Flush()
		case <-cpuT.C:
			c, err := cpu.Times(false)
			if err != nil {
				log.Printf("unable to get cpu: %s", err.Error())
				return
			}

			if _, err := fmt.Fprintf(w, "event:cpu\ndata:User: %.2f, Sys: %.2f, Idle: %.2f\n\n", c[0].User, c[0].System, c[0].Idle); err != nil {
				log.Printf("unable to write: %s", err.Error())
				return
			}

			rc.Flush()
		}
	}
	srv := r.Context().Value(http.ServerContextKey).(*http.Server)
	srv.Shutdown(context.Background())
}
