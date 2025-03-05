//@ts-check

//import * as echarts from 'echarts';

function zip(arr1, arr2) {
    return arr1.map((element, index) => [element, arr2[index]]);
}

// data was =zip(xAxisData, series1_data[0])
function chart_series(dataset) {
    return dataset.map(d => ({
        name: d.stack,
        type: 'line',
        data: d.data,
        markPoint: {
            data: [
                {
                    name: 'Max',
                    type: 'max'
                }
            ],
            label: {
                show: true,
                formatter: function (params) {
                    return formatBytes(params.value, 'bps',
                        2)
                }
            }
        }
    }))
}
function option_testchart(title, yAxisName, series_data) {
    return {
        grid: {
            left: '3%',
            right: '4%',
            bottom: '3%',
            containLabel: true
        },
        legend: {
            show: true,
            align: 'left',
            type: 'scroll',
            //   orient: 'vertical'
        },
        series: series_data,
        title: {
            text: title
        },
        toolbox: {
            show: true,
            feature: {
                saveAsImage: {},
                dataZoom: {
                    yAxisIndex: 'none'
                },
                magicType: { type: ['line', 'bar'] },
                restore: {},
                dataView: {
                    show: true,
                    lang: null,
                    backgroundColor: ""
                }
            }
        },
        dataZoom: [
            {
                type: 'inside',
                start: 0,
                end: 100
            },
            {
                start: 0,
                end: 100
            }
        ],

        tooltip: {
            show: true,
            trigger: 'axis',
            formatter: function (params) {
                return formatTooltip(params)
            }
        },
        xAxis: [
            {
                type: 'time',
                name: 'Time',
            }
        ],
        yAxis: [
            {
                name: yAxisName,
                axisLabel: {
                    formatter: function (value, index) {
                        return formatBytes(value, 'bps',
                            2)
                    },
                    showMinLabel: null,
                    showMaxLabel: null
                }
            }
        ]
    }
}
function addEChart(canvas, theme, xscales, datasets, labels) {
    // loop over chart
    let goecharts_testchart = echarts.init(canvas,theme);
    let data = chart_series(datasets);
    let opt = option_testchart("NSPEED", "Download", data)
    goecharts_testchart.setOption(opt);
    goecharts_testchart.group = 'group1'; // group to sync mouse
    console.log(opt);
    return goecharts_testchart
}
function genChart(canvas, theme, jsonreport) {
    // results is an array of "result"
    var results = jsonreport.Results;
    var chartType = (jsonreport.samplerate === 0) ? "line" : "bar";
    var xscales = {
        type: 'time',
        title: {
            display: true,
            text: "time"
        }
    };
    var labels = {}
    var datasets = [];
    if (jsonreport.samplerate === 0) {
        labels = results.map(r => r.id); //[... new Set(results.map(r => r.batchname))];
        xscales = {
            stacked: true,
            type: 'category',
            title: {
                display: false,
                text: ""
            }
        };
        datasets = [{
            label: 'Dataset 1',
            data: results.map((e, i) => ({ x: e.id, y: e.rxbps + e.txbps }))
        }];
    } else {
        datasets = results.map((result, i) => ({
            fill: true,
            hoverBorderColor: '#000',
            hoverBorderRadius: 2,
            hoverBorderWidth: 2,
            interpolate: true,
            label: (jsonreport.samplerate === 0) ? result.id : result.Samples.map(e => e.date),
            order: i,
            pointRadius: 2,
            showLine: true,
            stack: result.gid,
            data: result.Samples.map(e => ([new Date(e.date), (e.rxbytes + e.txbytes) * 8.0 / e.duration ]))
        }));
    }
    var chart = addEChart(canvas, theme, xscales, datasets, labels);
    return chart;
}