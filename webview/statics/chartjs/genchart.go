package chartjs

import (
	"bytes"
	"fmt"
	"text/template"
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
var {{.Name}} = new Chart(document.getElementById("{{.Name}}ID").getContext("2d"), {
	type: "{{.Type}}",
	options: {{.Options}},
	data: {
	  labels: {{.Labels}},
	  datasets: {{.Datasets}}
	}
  });
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

func GenChart(name, labels, datasets, options string) (string, error) {
	if options == "" {
		options = "{}"
	}

	var chart = struct {
		Name     string
		Type     string
		Labels   string
		Datasets string
		Options  string
	}{name, "line", labels, datasets, options}

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
