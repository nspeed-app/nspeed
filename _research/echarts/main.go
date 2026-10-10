package main

import (
	"fmt"
	"math/rand"
	"net/http"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/types"
)

const FormatBytes = `function formatBytes(value,index,precision=2) {
	if (value === 0) return '0 Bytes';
	const k = 1000;
	const dm = precision;
	const sizes = ['Bytes', 'kB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'];
	const i = Math.floor(Math.log(value) / Math.log(k));
	return parseFloat((value / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
}`

// generate random data for line chart
func generateLineItems() []opts.LineData {
	items := make([]opts.LineData, 0)
	for i := 0; i < 7; i++ {
		items = append(items, opts.LineData{Value: rand.Intn(30000)})
	}
	return items
}

func httpserver(w http.ResponseWriter, _ *http.Request) {
	// create a new line instance
	line := charts.NewLine()
	// set some global options like Title/Legend/ToolTip or anything else
	line.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{Theme: types.ThemeWesteros}),
		charts.WithTitleOpts(opts.Title{
			Title:    "Line example in Westeros theme",
			Subtitle: "Line chart rendered by the http server this time",
		}),
		charts.WithYAxisOpts(opts.YAxis{
			// carefull, passsing double-quotes in js will break (they're escaped)
			AxisLabel: &opts.AxisLabel{Formatter: opts.FuncOpts("function (value, index) { return value + ' - ' + index }")},
		}),
	)

	// Put data into instance
	line.SetXAxis([]string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}).
		AddSeries("Category A", generateLineItems()).
		AddSeries("Category B", generateLineItems()).
		SetSeriesOptions(charts.WithLineChartOpts(opts.LineChart{Smooth: true}))
	line.Render(w)
}

func main() {
	http.HandleFunc("/", httpserver)
	fmt.Println("listening on http://localhost:7777")
	http.ListenAndServe(":7777", nil)
}
