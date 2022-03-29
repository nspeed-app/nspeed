import { } from "https://cdnjs.cloudflare.com/ajax/libs/Chart.js/2.9.3/Chart.min.js";
import { } from "https://cdn.jsdelivr.net/npm/chartjs-plugin-crosshair@1.1.6/dist/chartjs-plugin-crosshair.min.js";

function generateDataset(shift, label, color) {
  var data = [];
  var x = 0;

  while (x < 30) {
    data.push({ x: x, y: Math.sin(shift + x / 3) });
    x += Math.random();
  }

  var dataset = {
    backgroundColor: color,
    borderColor: color,
    showLine: true,
    fill: false,
    pointRadius: 2,
    label: label,
    data: data,
    lineTension: 0,
    interpolate: true
  };
  return dataset;
}

const options = {
  plugins: {
    crosshair: {
      sync: {
        enabled: true
      }
    }
  },

  tooltips: {
    mode: "interpolate",
    intersect: false,
    callbacks: {
      title: function (a, d) {
        return a[0].xLabel.toFixed(2);
      },
      label: function (i, d) {
        return d.datasets[i.datasetIndex].label + ": " + i.yLabel.toFixed(2);
      }
    }
  }
};


var chart1 = new Chart(document.getElementById("chart1").getContext("2d"), {
  type: "scatter",
  options,
  data: {
    datasets: [
      generateDataset(0, "A", "red"),
      generateDataset(1, "B", "green"),
      generateDataset(2, "C", "blue")
    ]
  }
});

var chart2 = new Chart(document.getElementById('chart2').getContext("2d"), {
  type: "scatter",
  options,
  data: {
    datasets: [
      generateDataset(4, "G", "cyan"),
      generateDataset(2, "H", "orange"),
      generateDataset(1, "I", "teal")
    ]
  }
});



