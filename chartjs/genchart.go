package chartjs

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"text/template"

	"github.com/rs/zerolog/log"
	"nspeed.app/metric"
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
	fill: true,
	pointRadius: 2,
	label: {{.Label}},
	data: {{.Data}},
	//hoverBackgroundColor: '#666',
	hoverBorderColor: '#0000',
	hoverBorderWidth: 2,
	hoverBorderRadius: 2,
	interpolate: true,
	stack: {{.Stack}}
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

func GenChart(name, chartType, datasets, options string) (string, error) {
	if options == "" {
		options = "{}"
	}

	var chart = struct {
		Name     string
		Type     string
		Datasets string
		Options  string
	}{name, chartType, datasets, options}

	var out bytes.Buffer
	err := Templates[chartTmplName].Execute(&out, chart)
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

func GenDataset(color, label, data, stack string) (string, error) {
	var dataset = struct {
		Color string
		Label string
		Data  string
		Stack string
	}{color, label, data, stack}

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

	dsl := make([]string, 0, len(jsons))
	for i := range jsons {
		is := strconv.Itoa(i)
		ds, err := GenDataset("", "jsonfile["+is+"].id", "data"+is, "jsonfile["+is+"].gid")
		if err != nil {
			log.Panic().Err(err).Send()
		}
		dsl = append(dsl, ds)
	}
	options := `{
		interaction: {
			intersect: false,
			mode: 'index',
		},
		plugins: {
			legend:{
				onHover: onLegendHover
			},
			tooltip: {
                callbacks: {
                    label: TTlabel,
					footer: TTfooter
                }
            }
		},
		scales: {
            x: {
                type: 'time',
				title: {
					display: true,
					text : "time"
				},
            },
            y: {
				stacked: false,
				title: {
					display: true,
					text : "throughput"
				},
				ticks: {
				callback: function(item) {
					return formatBytes(item);
                }
			}
            }
		}
    }`

	s, err := GenChart("a", "line", JSArray(dsl...), options)
	if err != nil {
		log.Panic().Err(err).Send()
	}

	html := htmlHeader + strings.Join(utils.Map(js, addScript), "\n") + fromSamples(jsons) + s + htmlFooter
	//fmt.Println(fromSamples(jsons) + s)

	return html
}

func fromSamples(jsons []string) string {
	r := "<script> var jsonfile = " + JSArray(jsons...)
	for i := range jsons {
		is := strconv.Itoa(i)
		r += `
	 var labels` + is + ` = jsonfile[` + is + `].Samples.map(function(e) {
 		return e.date;
     });
     var data` + is + ` = jsonfile[` + is + `].Samples.map(function(e) {
  	     return {x: e.date, y: (e.rxbytes+e.txbytes)*8.0/e.duration} ;
     });
	`
	}
	r += "</script>"
	return r
}

func GenerateHTMLReport(htmlFilename string, wcWorkers *metric.Metrics, ungroup bool) {
	var out *utils.SmartFile
	var err error
	if htmlFilename != ":" {
		out, err = utils.FileCreate(htmlFilename)
		if err != nil {
			log.Error().Msgf("could not create html file (%s): %v", htmlFilename, err)
			return
		}
	}
	html := ChartJSON(metric.GenerateJSONReports(wcWorkers, ungroup))
	if out != nil {
		html, err = utils.InlineAssets(html, StaticFiles, true)
		if err != nil {
			log.Fatal().Msgf("could inline html file (%s): %v", htmlFilename, err)
		}
		_, err = out.WriteString(html)
		if err != nil {
			log.Fatal().Msgf("could not write to html file (%s): %v", htmlFilename, err)
		}
		log.Info().Msgf("html generated to %s", htmlFilename)
	} else {
		html, err = utils.InlineAssets(html, StaticFiles, false)
		if err != nil {
			log.Fatal().Msgf("could inline html file (%s): %v", htmlFilename, err)
		}
		err := utils.OpenHTML(html)
		if err != nil {
			log.Fatal().Msgf("could not open html: %v", err)
		}
	}
}
