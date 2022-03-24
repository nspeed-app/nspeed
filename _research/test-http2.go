package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

func main() {
	response, err := http.Get("https://http2.golang.org/reqinfo")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("is HTTP2: %v (%s)\n\n", response.ProtoAtLeast(2, 0), response.Proto)
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(body))
}
