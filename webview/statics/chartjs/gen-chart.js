//@ts-check

// import { Chart } from "chart.js";

function addChart(elem, type, xscales, datasets, labels) {
    console.log("addChart:",type,xscales,datasets,labels)
    var data = {
        datasets: datasets
    };

    if (labels !== undefined) {
        data.labels = labels;
    }
    var chart = new Chart(elem, {
        type: type,
        options: {
            responsive: true,
            interaction: {
                intersect: false,
                mode: 'nearest',
            },
            plugins: {
                legend: {
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
                x: xscales,
                y: {
                    stacked: true,
                    title: {
                        display: true,
                        text: "throughput"
                    },
                    ticks: {
                        callback: function (item) {
                            return formatBytes(item) + "bps";
                        }
                    }
                }
            }
        },
        data: data
    });
    console.log(data);
    return chart;
}

function genChart(canvas, jsonreport) {
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
            data : results.map((e, i) => ({x: e.id, y: e.rxbps + e.txbps}))
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
            data: result.Samples.map(e => ({ x: e.date, y: (e.rxbytes + e.txbytes) * 8.0 / e.duration }))
        }));
    }
    var chart = addChart(canvas, chartType, xscales, datasets, labels);
    return chart;
}