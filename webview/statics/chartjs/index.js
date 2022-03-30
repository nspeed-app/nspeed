import { } from "./chart.min.js";
import { } from "./chartjs-plugin-crosshair.min.js";


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
    tooltip: {
      animation: false,
      mode: "interpolate",
      intersect: false,
      callbacks: {
        title: function(a, d) {
          return a[0].element.x.toFixed(2);
        },
        label: function(d) {
          return (
            d.chart.data.datasets[d.datasetIndex].label + ": " + d.element.y.toFixed(2)
          );
        }
      }
    },
    crosshair: {
      sync: {
        enabled: true
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



