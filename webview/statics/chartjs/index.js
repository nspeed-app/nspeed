import { } from "./chart.min.js";
import { } from "./chartjs-plugin-crosshair.min.js";
import { } from "./chartjs-plugin-autocolors.js";
//import { }  from "https://cdn.jsdelivr.net/npm/chartjs-plugin-datalabels@2.0.0";

const autocolors = window['chartjs-plugin-autocolors'];


var x = 1; //minutes interval
var times = []; // time array
var tt = 0; // start time
var ap = ['AM', 'PM']; // AM-PM
var minutes = 0.1

//loop to increment the time and push results in array
for (var i = 0; tt < minutes * 60; i++) {
  var hh = Math.floor(tt / 60); // getting hours of day in 0-24 format
  var mm = (tt % 60); // getting minutes of the hour in 0-55 format
  times[i] = ("0" + (hh % 12)).slice(-2) + ':' + ("0" + mm).slice(-2) + ap[Math.floor(hh / 12)]; // pushing data in array in [00:00 - 12:00 AM/PM format]
  tt = tt + x;
}
var labels = times
//console.log(times);

function generateDataset(shift, label, color) {
  var data = [];
  var x = 0;

  for (let y in labels) {
    data.push(Math.random())
  }

  var dataset = {
    backgroundColor: color,
    borderColor: color,
    showLine: true,
    fill: false,
    pointRadius: 2,
    label: label,
    data: data,
    interpolate: true
  };
  return dataset;
}

const options = {
  responsive: true,
  interaction: {
    mode: 'index',
    intersect: false,
  },
  plugins: {
    // tooltip: {
    //   animation: false,
    //   mode: "interpolate",
    //   intersect: false,
    //   callbacks: {
    //     title: function(context) {
    //       let tti = context[0]
    //       console.log(tti)
    //       return ([tti.label])
    //     },
    //     label: function(context) {
    //       let label = context.dataset.label || '';
    //       if (label) {
    //         label += ': ';
    //       }
    //       if (context.parsed.y !== null) {
    //         label += context.parsed.y.toFixed(2)
    //       }
    //       return label;
    //     }
    //   }
    // },
    crosshair: {
      sync: {
        enabled: true
      }
    },
    autocolors: {
      mode: 'dataset'
    }
  }
};

Chart.register(autocolors);
//Chart.register(ChartDataLabels);

var chart1 = new Chart(document.getElementById("chart1").getContext("2d"), {
  type: "line",
  options,
  data: {
    labels: labels,
    datasets: [
      generateDataset(0, "A", "red"),
      generateDataset(1, "B", "green"),
      generateDataset(2, "C", "blue")
    ]
  }
});

var chart2 = new Chart(document.getElementById('chart2').getContext("2d"), {
  type: "line",
  options,
  data: {
    labels: labels,
    datasets: [
      generateDataset(4, "G", "cyan"),
      generateDataset(2, "H", "orange"),
      generateDataset(1, "I", "teal")
    ]
  }
});



