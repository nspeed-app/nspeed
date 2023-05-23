package main

import (
	"fmt"
	"log"

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

	utils.OpenHTML(html)
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
