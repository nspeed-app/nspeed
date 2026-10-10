package main

import (
	"io"
	"log"
	"os"
	"strings"

	"nspeed.app/utils"
	"nspeed.app/webview/statics/chartjs"
)

var js = []string{
	"https://cdnjs.cloudflare.com/ajax/libs/Chart.js/3.5.1/chart.js",
	"https://cdnjs.cloudflare.com/ajax/libs/date-fns/1.30.1/date_fns.js",
	"https://cdn.jsdelivr.net/npm/chartjs-adapter-date-fns/dist/chartjs-adapter-date-fns.bundle.min.js",
}

func addScript(src string) string {
	return `
 		<script src="` + src + `"></script>
`
}

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

	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Panic(err)
	}
	ds1, err := chartjs.GenDataset("'#4DC9F6'", "TEST", "data")
	if err != nil {
		log.Panic(err)
	}

	optionScales := `{
        scales: {
            x: {
                type: 'timeseries',
            }
        }
    }`

	s, err := chartjs.GenChart("a", "bar", "labels", chartjs.JSArray(ds1), optionScales)
	if err != nil {
		log.Panic(err)
	}

	html := htmlHeader + strings.Join(utils.Map(js, addScript), "\n") + fromSamples(string(stdin)) + s + htmlFooter
	//fmt.Println(html)

	utils.OpenHTML(html)
}

func fromSamples(json string) string {
	r := "<script> var jsonfile = " + json
	r += `
	 var labels = jsonfile[0].Samples.map(function(e) {
 		return e.date;
     });
     var data = jsonfile[0].Samples.map(function(e) {
  	     return e.rxbytes;
     });
	`
	r += "</script>"
	return r
}
