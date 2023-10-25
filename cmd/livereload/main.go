package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
)

//go:embed index.html
var indexHTML []byte

var filename string

func init() {
	flag.StringVar(&filename, "f", "", "Name of the file to use")
	flag.Parse()
	err := loadHTML()
	if err != nil {
		log.Panicf("init failed on loading html file: %s. Error is %s\n", filename, err)

	}
}

func loadHTML() error {
	if filename == "" {
		return nil
	}

	html, err := os.ReadFile(filename)

	if err != nil {
		return fmt.Errorf("cant read %s: %w", filename, err)
	}

	indexHTML = html
	return nil
}

func main() {
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		log.Println("incoming /ws from", r.RemoteAddr)
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)

		if err != nil {
			log.Println("ws upgrade failed", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		go func() {
			for {
				_, _, err := conn.ReadMessage()
				if err != nil {
					if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
						log.Println("connection closed")
					} else {
						log.Println("error reading message:", err)
					}
					break
				}
			}
		}()

		err = conn.WriteMessage(websocket.TextMessage, []byte("hello world"))
		if err != nil {
			log.Println("error on WriteMessage in /ws handler", err)
		}

	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Println("incoming / from", r.RemoteAddr)
		if r.URL.Query().Get("reload") == "true" {
			log.Println("reloading")
			err := loadHTML()
			if err != nil {
				log.Println(err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		_, err := w.Write(indexHTML)
		if err != nil {
			log.Println("error on Write in / handler", err)
		}
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}
