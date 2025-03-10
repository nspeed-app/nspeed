// @ts-check

//import {echarts} from 'echarts';

function zip(arr1, arr2) {
    return arr1.map((element, index) => [element, arr2[index]]);
}

var yMax = "dataMax";

function chart_series(dataset) {
    return dataset.map(d => ({
        name: d.id,
        stack: d.stack,
        type: 'line',
        data: d.data,
        smooth: true,
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

function chart_single(dataset) {
    return dataset.map(d => ({
        name: d.id,
        stack: d.stack,
        type: 'bar',
        data: d.data,
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
                type: 'slider',
                filterMode: 'none'
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
                max: yMax,
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
function addEChart(container, theme, xscales, datasets, labels) {
    // loop over chart
    let c = echarts.init(container, theme);
    let data = chart_series(datasets);
    let opt = option_testchart("TODO", "bps", data)
    opt.xAxis = [xscales];
    c.setOption(opt);
    c.group = 'group1'; // group to sync mouse
    console.log("INIT:", c)
    return c
}
/**
 * @param {HTMLElement} container
 * @param {any} theme
 * @param {boolean} hasSeries
 * @param {any[]} results
 */
function genChart(container, theme, hasSeries, results) {
    console.log("hasSeries", hasSeries);
    console.log("results", results);
    var xscales = {
        type: 'time',
        title: {
            display: true,
            text: "Time"
        }
    };
    var labels = {}
    var datasets = [];

    if (!hasSeries) {
        labels = Array.from(new Set(results.map(r => r.batchname))); //[... new Set(results.map(r => r.batchname))];
        xscales = {
            stacked: true,
            type: 'category',
            title: {
                display: true,
                text: "batch"
            }
        };
        datasets = [{
            label: "TODO",
            data: results.map((e, i) => (e.rxbps + e.txbps))
        }];
    } else {
        datasets = results.map((result, i) => ({
            fill: true,
            hoverBorderColor: '#000',
            hoverBorderRadius: 2,
            hoverBorderWidth: 2,
            interpolate: true,
            label: result.Samples.map(e => e.date), // (jsonreport.samplerate === 0) ? result.id : 
            order: i,
            pointRadius: 2,
            showLine: true,
            stack: result.gid,
            id: result.id,
            data: result.Samples.map(e => ([new Date(e.date), (e.rxbytes + e.txbytes) * 8.0 / e.duration]))
        }));
        console.log("dataset", datasets);
    }
    return addEChart(container, theme, xscales, datasets, labels);
}
/**
 * @param {HTMLElement} container
 * @param {string} theme
 * @param {any} jsonreport
 */
function genCharts(container, theme, jsonreport) {
    // results is an array of "result"
    var results = jsonreport.Results;
    // todo: change json to group by batch
    // this does  "uniq" on batch , carefull of inversion.
    var batches = Array.from(new Set(results.map(r => (r.batch, r.batchname))));
    console.log("batches", batches);
    console.log("results", results);

    var degroup = false;
    if (degroup) {
        // create a div per chart
        var divs = batches.map((b, bn) => {
            var element = document.createElement("div");
            element.id = "Batch" + bn;
            element.style = "width: " + 100 / batches.length + "%";
            container.appendChild(element);
            return element;
        });

        // loop thru batches, adding a chart per batch
        var charts = batches.map((b, bn) => {
            return genChart(divs[bn], theme, (results.samplerate != 0), results.filter(r => r.batch == bn));
        });
    } else {
        var element = document.createElement("div");
        element.id = "All Batchs";
        element.style = "width: 100%";
        container.appendChild(element);
        return genChart(element, theme, (results.samplerate != 0), results);
    }
    echarts.connect('group1');
    return charts;
}