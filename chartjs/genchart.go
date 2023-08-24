package chartjs

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"strconv"
	"strings"
	"text/template"

	"nspeed.app/utils"
)

var js = []string{
	"https://cdnjs.cloudflare.com/ajax/libs/Chart.js/4.4.0/chart.umd.min.js",
	"https://cdnjs.cloudflare.com/ajax/libs/date-fns/1.30.1/date_fns.js",
	"https://cdn.jsdelivr.net/npm/chartjs-adapter-date-fns/dist/chartjs-adapter-date-fns.bundle.min.js",
	"nspeed.js",
}

//go:embed statics
var staticFiles embed.FS

var StaticFiles fs.FS

func init() {
	var err error
	StaticFiles, err = fs.Sub(staticFiles, "statics")
	if err != nil {
		panic("jschart: failed to get embeded static filesystem: " + err.Error())
	}
}
func addScript(src string) string {
	return `
 		<script src="` + src + `"></script>
`
}

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
	// backgroundColor: {{.Color}},
	// borderColor: {{.Color}},
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

const htmlHeader = `
<html>
<head>
</head>
	<body>
`
const htmlFooter = `
	</body>
</html>`

func ChartJSON(jsons []string) string {
	var dsl []string = make([]string, 0, len(jsons))
	for i := range jsons {
		is := strconv.Itoa(i)
		ds, err := GenDataset("", "jsonfile["+is+"].command", "data"+is)
		if err != nil {
			log.Panic(err)
		}
		dsl = append(dsl, ds)
	}
	optionScales := `{
        scales: {
            x: {
                type: 'timeseries',
				stacked: true
            },
            y: {
				stacked: true
            }
		}
    }`

	s, err := GenChart("a", "bar", "labels", JSArray(dsl...), optionScales)
	if err != nil {
		log.Panic(err)
	}

	html := htmlHeader + strings.Join(utils.Map(js, addScript), "\n") + fromSamples(jsons) + s + htmlFooter
	fmt.Println(html)

	return html
}

func fromSamples(jsons []string) string {
	r := "<script> var jsonfile = " + JSArray(jsons...)
	for i := range jsons {
		is := strconv.Itoa(i)
		r += `
	 var labels = jsonfile[` + is + `].Samples.map(function(e) {
 		return e.date;
     });
     var data` + is + ` = jsonfile[` + is + `].Samples.map(function(e) {
  	     return e.rxbytes*8.0/e.duration;
     });
	`
	}
	r += "</script>"
	return r
}
