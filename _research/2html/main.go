package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"

	"nspeed.app/utils"
	"nspeed.app/webview/statics/chartjs"
)

const js = "https://cdn.jsdelivr.net/npm/chart.js"

const htmlHeader = `
<html>
<head>
</head>
	<body>
`
const htmlFooter = `
	</body>
</html>`

func main() {
	ds1, err := chartjs.GenDataset("'#4DC9F6'", "# of Votes", "[12, 19, 3, 5, 2, 3]")
	if err != nil {
		log.Panic(err)
	}
	ds2, err := chartjs.GenDataset("'#00a950'", "# of Non Votes", "[1, 2, 48, 2, 12, 43]")
	if err != nil {
		log.Panic(err)
	}
	s, err := chartjs.GenChart("a", "['Red', 'Blue', 'Yellow', 'Green', 'Purple', 'Orange']", chartjs.JSArray(ds1, ds2), "")
	if err != nil {
		log.Panic(err)
	}

	html := htmlHeader + "<script src=\"" + js + "\"></script>" + s + htmlFooter
	fmt.Println(html)

	getRoot := func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, html)
	}
	http.HandleFunc("/", getRoot)

	ln, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("serving at http://", ln.Addr().String())
	go func() {
		utils.Openbrowser("http://"+ln.Addr().String(), nil, nil)
	}()
	_ = http.Serve(ln, http.DefaultServeMux)

}
