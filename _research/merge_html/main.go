package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"nspeed.app/utils"
)

func main() {
	filename := os.Args[1]
	html, err := os.ReadFile(filename)
	if err != nil {
		log.Fatal(err)
	}
	s, err := utils.InlineScript(string(html), filepath.Dir(filename))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(s)
}
