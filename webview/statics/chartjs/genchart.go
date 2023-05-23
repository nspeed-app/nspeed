package chartjs

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"text/template"

	"nspeed.app/utils"
)

// step 1
// html:
// <div class="split"><canvas id="chartID"></canvas></div>

// step 2:
// js:

// template names
const (
	chartTmplName   = "chart"
	datasetTmplName = "dataset"
)

// template files
const (
	chartTmplText = `
	<div>
 	 <canvas id='{{.Name}}ID'></canvas>
	</div>
	<script>
 var {{.Name}} = new Chart(document.getElementById('{{.Name}}ID'), {
	type: '{{.Type}}',
	options: {{.Options}},
	data: {
	  labels: {{.Labels}},
	  datasets: {{.Datasets}}
	}
  });
  </script>

`
	datasetTmplText = `
{
	backgroundColor: {{.Color}},
	borderColor: {{.Color}},
	showLine: true,
	fill: false,
	pointRadius: 2,
	label: {{.Label}},
	data: {{.Data}},
	interpolate: true
}`
)

// template names to files
var templateFiles = map[string]string{
	chartTmplName:   chartTmplText,
	datasetTmplName: datasetTmplText,
}

// template names to Template
var Templates map[string]*template.Template

func init() {
	Templates = make(map[string]*template.Template, len(templateFiles))
	for k, v := range templateFiles {
		chartTmpl, err := template.New(k).Parse(v)
		if err != nil {
			fmt.Println("template name:", k)
			panic(err)
		}
		Templates[k] = chartTmpl
	}
}

func GenChart(name, chartType, labels, datasets, options string) (string, error) {
	if options == "" {
		options = "{}"
	}

	var chart = struct {
		Name     string
		Type     string
		Labels   string
		Datasets string
		Options  string
	}{name, chartType, labels, datasets, options}

	var out bytes.Buffer
	err := Templates[chartTmplName].Execute(&out, chart)
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

func GenDataset(color, label, data string) (string, error) {
	var dataset = struct {
		Color string
		Label string
		Data  string
	}{color, label, data}

	var out bytes.Buffer
	err := Templates[datasetTmplName].Execute(&out, dataset)
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

// label: array of string of time value
// data: data of the unit (scale to formatBytes)

func JSArray(vals ...string) string {
	return "[" + strings.Join(vals, ",") + "]"
}

func JSString(s string) string {
	return "'" + s + "'"
}

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

func ChartJSON(json string) string {

	ds1, err := GenDataset("'#4DC9F6'", "jsonfile[0].command", "data")
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

	s, err := GenChart("a", "bar", "labels", JSArray(ds1), optionScales)
	if err != nil {
		log.Panic(err)
	}

	html := htmlHeader + strings.Join(utils.Map(js, addScript), "\n") + fromSamples(json) + s + htmlFooter
	//fmt.Println(html)

	return html
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
