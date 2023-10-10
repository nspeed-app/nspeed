package chartjs

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"text/template"
	"time"

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
		{{ if .Labels}}
		labels: {{.Labels}},
		{{- end}}
		datasets: {{.Datasets}}
	}
	});
  </script>`
	datasetTmplText = `
{
	{{ range $key, $value := .Options }}
	{{ $key }}: {{ $value }},
 	{{- end }}
	label: {{.Label}},
	data: {{.Data}}
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

func GenChart(name, chartType, datasets, labels, options string) (string, error) {
	if options == "" {
		options = "{}"
	}

	var chart = struct {
		Name     string
		Type     string
		Datasets string
		Labels   string
		Options  string
	}{name, chartType, datasets, labels, options}

	var out bytes.Buffer
	err := Templates[chartTmplName].Execute(&out, chart)
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

func GenDataset(color, label, data string, options map[string]string) (string, error) {
	var dataset = struct {
		Color   string
		Label   string
		Data    string
		Options map[string]string
	}{color, label, data, options}

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

// build a chart from nspeed json format
func ChartJSON(jsonData string) string {
	if jsonData == "" {
		return ""
	}
	report, err := metric.ReportFromJSON(jsonData)
	if err != nil {
		return ""
	}
	tick := time.Duration(report.SampleRate)
	results := report.Results

	// build and sort batch names
	// batchNamesSet := utils.NewSet[string]()
	// for _, r := range results.Metrics {
	// 	batchNamesSet.Add(JSString(results.Batches[r.Batch]))
	// }
	// batchNames := batchNamesSet.Iter()
	// slices.Sort(batchNames)

	dsl := make([]string, 0, len(results))
	for i := range results {
		is := strconv.Itoa(i)
		options := make(map[string]string)

		stack := "jsondata[" + is + "].gid"
		if tick == 0 {
			stack = "jsondata[" + is + "].batch"
		}
		options["order"] = is
		options["stack"] = stack
		options["showLine"] = "true"
		options["fill"] = "true"
		options["pointRadius"] = "2"
		options["hoverBorderColor"] = "'#000'"
		options["hoverBorderWidth"] = "2"
		options["hoverBorderRadius"] = "2"
		options["interpolate"] = "true"

		idx := "jsondata[" + is + "]"
		data := ""
		if tick == 0 {
			data = "[{x: " + idx + ".batchname, y: (" + idx + ".rxbytes+" + idx + ".txbytes)*8.0/" + idx + ".duration}]"
		} else {
			options["label"] = idx + ".Samples.map(e => e.date)"
			data = idx + ".Samples.map(e => ({x: e.date, y: (e.rxbytes+e.txbytes)*8.0/e.duration}))"
		}
		ds, err := GenDataset("", idx+".id", data, options)
		if err != nil {
			log.Panic().Err(err).Send()
		}
		dsl = append(dsl, ds)
	}
	options := `{
		responsive: true,
		interaction: {
			intersect: false,
			mode: 'nearest',
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
		},`
	sType := ""
	labels := ""
	if tick != 0 {
		options += `
		scales: {
            x: {
                type: 'time',
				title: {
					display: true,
					text : "time"
				},
            },
            y: {
				stacked: true,
				title: {
					display: true,
					text : "throughput"
				},
				ticks: {
				callback: function(item) {
					return formatBytes(item) +"bps";
                }
			}
            }
		}
    }`
		sType = "line"
		labels = ""
	} else {
		options +=
			`
			scales: {
				x: {
					type: 'category',
					title: {
						display: false,
					},
				},
				y: {
					stacked: true,
					title: {
						display: true,
						text : "throughput"
					},
					ticks: {
					callback: function(item) {
						return formatBytes(item) +"bps";
					}
				}
				}
			}
		}`
		sType = "bar"
		labels = "jsondata.map(r => r.batchname)"
	}

	s, err := GenChart("a", sType, JSArray(dsl...), labels, options)
	if err != nil {
		log.Panic().Err(err).Send()
	}

	html := htmlHeader + strings.Join(utils.Map(js, addScript), "\n") + fromSamples(jsonData, &report) + s + htmlFooter
	//fmt.Println(fromSamples(jsons) + s)

	return html
}

func fromSamples(jsonData string, report *metric.Report) string {
	r := "<script> var jsonreport = " + jsonData + ";\n"
	r += "var jsondata = jsonreport.Results;\n"
	for i := range report.Results {
		is := strconv.Itoa(i)
		if report.SampleRate == 0 {
			r += `
var data` + is + ` = [{x: jsondata[` + is + `].batchname, y: (jsondata[` + is + `].rxbytes+jsondata[` + is + `].txbytes)*8.0/jsondata[` + is + `].duration}];`
		} else {
			r += `
var labels` + is + ` = jsondata[` + is + `].Samples.map(e => e.date);
var data` + is + ` = jsondata[` + is + `].Samples.map(e => ({x: e.date, y: (e.rxbytes+e.txbytes)*8.0/e.duration}));`
		}
	}
	r += "\n</script>"
	return r
}

func GenerateHTMLReport(htmlFilename string, jsData string) {
	var out *utils.SmartFile
	var err error
	if htmlFilename != ":" {
		out, err = utils.FileCreate(htmlFilename)
		if err != nil {
			log.Error().Msgf("could not create html file (%s): %v", htmlFilename, err)
			return
		}
	}
	html := ChartJSON(jsData)
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
