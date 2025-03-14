package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"nspeed.app/utils"
	"nspeed.app/webview"
)

func main() {

	flag.CommandLine.Usage = func() {
		name := "json2html" //todo
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n\n", name)
		fmt.Fprintf(flag.CommandLine.Output(), "  %s jsonfile htmlfile\n", name)
	}

	flag.Parse()
	if flag.NArg() != 2 {
		flag.CommandLine.Usage()
		os.Exit(10)
	}

	//read json
	jsonFile, err := utils.OpenURI(flag.Arg(0))
	if err != nil {
		log.Fatalf("cannot open input file %s: %v", flag.Arg(0), err)
		return
	}

	json, err := io.ReadAll(jsonFile)
	if err != nil {
		log.Fatalf("cannot read input file: %v", err)
		return
	}

	webview.GenerateHTMLReport(context.Background(), flag.Arg(1), string(json))
}
