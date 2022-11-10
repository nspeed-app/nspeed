package main

import (
	"fmt"
	"io"
	"log"
	"strings"

	"net/http"
)

func main() {
	contentType := "text/plain"
	bodyString := "This is my request payload"
	contentLength := int64(len(bodyString))
	body := strings.NewReader(bodyString)

	req, err := http.NewRequest("POST", "https://www.google.com/", body)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = contentLength
	response, err := http.DefaultClient.Do(req)

	check(err)
	buffer, err := io.ReadAll(response.Body)
	check(err)
	fmt.Println("PUT response:", response.StatusCode, len(string(buffer)))

}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
