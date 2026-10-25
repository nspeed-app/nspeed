// @ts-check
// import echarts from './echarts.min.js';
/// <reference types="echarts" />

/**
 * @param {any[]} arr1
 * @param {{ [x: string]: any; }} arr2
 */
function zip(arr1, arr2) {
    return arr1.map((/** @type {any} */ element, /** @type {string | number} */ index) => [element, arr2[index]]);
}

var yMax = "dataMax";

/**
 * @param {any[]} dataset
 */
function chart_series(dataset) {
    return dataset.map((/** @type {{ id: any; stack: any; data: any; }} */ d) => ({
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
            ]
        }
        //     label: {
        //         show: true,
        //         formatter: function (/** @type {{ value: any; }} */ params) {
        //             return formatBytes(params.value, 'bps',
        //                 2)
        //         }
        //     }
        // }
    }))
}

/**
 * @param {any} labels
 * @param {any[]} dataset
 */
function chart_noseries(labels, dataset) {
    return dataset.map((/** @type {{ id: any; stack: any; data: any; }} */ d) => ({
        name: d.id,
        stack: d.stack,
        type: 'bar',
        data: d.data,
    }))
}

/**
 * @param {string} title
 * @param {string} yAxisName
 * @param {any} series_data
 * @param {boolean} hasSeries
 */
function option_testchart(title, yAxisName, series_data, hasSeries) {
    let opts = {
        grid: {
            left: '3%',
            right: '4%',
            bottom: 60, // leave room for the dataZoom slider below the x axis labels
            containLabel: true
        },
        legend: {
            show: true,
            top: 0, // echarts 6 defaults to bottom, where it overlaps the dataZoom slider
            align: 'left',
            type: 'scroll',
        },
        series: series_data,
        title: {
            text: title
        },
        toolbox: {
            show: true,
            feature: {
                saveAsImage: {},
                restore: {},
                dataView: {
                    show: false,
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
                    formatter: function (/** @type {any} */ value, /** @type {any} */ index) {
                        return formatBytes(value, 'bps', 2)
                    },
                    showMinLabel: null,
                    showMaxLabel: null
                }
            }
        ]
    }

    if (hasSeries) {
        opts.toolbox.feature.magicType = { type: ['line', 'bar'] };
        opts.tooltip.formatter = formatSeriesTooltip;
    } else {
        opts.tooltip.formatter = formatTooltip;
    }
    return opts
}
/**
 * @param {HTMLElement} container
 * @param {string} theme
 * @param {boolean} hasSeries
 * @param {any} xscales
 * @param {any} datasets
 */
function addEChart(container, theme, hasSeries, xscales, datasets) {
    // loop over chart
    let c = echarts.init(container, theme);
    let data = hasSeries ? chart_series(datasets) : datasets;
    let opt = option_testchart("", "bps", data, hasSeries)
    opt.xAxis = [xscales];
    c.setOption(opt);
    c.group = 'group1'; // group to sync mouse
    console.log("INIT:", c)
    return c
}
/**
 * @param {HTMLElement} container
 * @param {string} theme
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
            type: 'category',
            title: {
                display: true,
                text: "batch"
            },
            data: labels
        };
        datasets = results.map((r, i) => {
            console.log("i", i, "r", r);
            return {
                type: 'bar',
                name: r.gid,
                stack: r.batchname,
                data: labels.map((/** @type {any} */ l) => {
                    if (l == r.batchname) {
                        return r.rxbps + r.txbps;
                    }
                    return;
                })
            };
        }); //[{
        //     label: "TODO",
        //     data: results.map((e, i) => (e.rxbps + e.txbps))
        // }];
    } else {
        datasets = results.map((result, i) => ({
            fill: true,
            hoverBorderColor: '#000',
            hoverBorderRadius: 2,
            hoverBorderWidth: 2,
            interpolate: true,
            label: result.Samples.map((/** @type {{ date: any; }} */ e) => e.date), // (jsonreport.samplerate === 0) ? result.id : 
            order: i,
            pointRadius: 2,
            showLine: true,
            stack: result.gid,
            id: result.id,
            data: result.Samples.map((/** @type {{ date: string | number | Date; rxbytes: any; txbytes: any; duration: number; }} */ e) => ([new Date(e.date), (e.rxbytes + e.txbytes) * 8.0 / e.duration]))
        }));
        console.log("dataset", datasets);
    }
    return addEChart(container, theme, hasSeries, xscales, datasets);
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
    var batches = Array.from(new Set(results.map((/** @type {{ batch: any; batchname: any; }} */ r) => (r.batch, r.batchname))));
    console.log("batches", batches);
    console.log("results", results);

    var degroup = false;
    charts = [];
    if (degroup) { // wip
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
            return genChart(divs[bn], theme, (jsonreport.samplerate != 0), results.filter((/** @type {{ batch: number; }} */ r) => r.batch == bn));
        });
    } else {
        var element = document.createElement("div");
        element.id = "All Batchs";
        element.style = "width: 100%";
        container.appendChild(element);
        console.log("samplerate", jsonreport.samplerate)
        charts.push(genChart(element, theme, (jsonreport.samplerate != 0), results));
    }
    echarts.connect('group1');
    return charts;
}