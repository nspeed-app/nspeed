package main

import (
	"io"
	"log"
	"os"

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

	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Panic(err)
	}
	ds1, err := chartjs.GenDataset("'#4DC9F6'", "TEST", "data")
	if err != nil {
		log.Panic(err)
	}

	s, err := chartjs.GenChart("a", "labels", chartjs.JSArray(ds1), "")
	if err != nil {
		log.Panic(err)
	}

	html := htmlHeader + "<script src=\"" + js + "\"></script>" + fromSamples(string(stdin)) + s + htmlFooter
	//fmt.Println(html)

	utils.OpenHTML(html)
}

func fromSamples(json string) string {
	r := "<script> var jsonfile = " + json
	r += `
	 var labels = jsonfile[0].Samples.map(function(e) {
 		return e.date;
     });
     var data = jsonfile[0].jsonarray.map(function(e) {
  	     return e.rxbytes;
     });
	`
	r += "</script>"
	return r
}

// var jsonfile = {
// 	"jsonarray": [{
// 	   "name": "Joe",
// 	   "age": 12
// 	}, {
// 	   "name": "Tom",
// 	   "age": 14
// 	}]
//  };

//  var labels = jsonfile.jsonarray.map(function(e) {
// 	return e.name;
//  });
//  var data = jsonfile.jsonarray.map(function(e) {
// 	return e.age;
//  });

//  var ctx = canvas.getContext('2d');
//  var config = {
// 	type: 'line',
// 	data: {
// 	   labels: labels,
// 	   datasets: [{
// 		  label: 'Graph Line',
// 		  data: data,
// 		  backgroundColor: 'rgba(0, 119, 204, 0.3)'
// 	   }]
// 	}
//  };
