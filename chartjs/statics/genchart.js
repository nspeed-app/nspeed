import { Chart } from "chart.js";
//@ts-check

function addChart(elem, type, xscales, datasets, labels) {
    var data = {
        datasets: data
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
    return chart;
}

function genChart(canvas, jsonreport, command) {
    command.textContent = jsonreport.command;
    // results is an array of "result"
    var results = jsonreport.Results;
    var chartType = (jsonreport.samplerate === 0) ? "line" : "bar";
    var xscales = {
        type: 'time',
        title: {
            display: true,
            text: "time"
        },
    };
    var labels = {}
    if (jsonreport.samplerate === 0) {
        labels = results.map(r => r.batchname);
        xscales = {
            type: 'category',
            title: {
                display: false,
                text: ""
            },
        };
    }
    var datasets = results.map((result, i) => ({
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
    var chart = addChart(canvas, chartType, datasets, labels);
}