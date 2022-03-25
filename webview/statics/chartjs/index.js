import {} from "./chart.min.js";
import {} from "./chartjs-plugin-crosshair.min.js";

const labels = [
  'January',
  'February',
  'March',
  'April',
  'May',
  'June',
];

const data1 = {
  labels: labels,
  datasets: [{
    label: 'dataset 1',
    backgroundColor: 'rgb(255, 99, 132)',
    borderColor: 'rgb(255, 99, 132)',
    data: [0, 10, 5, 2, 20, 30, 45],
  }]
};

const data2 = {
  labels: labels,
  datasets: [{
    label: 'dataset 2',
    backgroundColor: 'rgb(255, 99, 132)',
    borderColor: 'rgb(0, 99, 132)',
    data: [0, 3, 5, 20, 1, 3, 5],
  }]
};

const config1 = {
  type: 'line',
  data: data1,
  options: {
    interaction: {
      intersect: false,
      mode: 'x',
    },
  }
};

const config2 = {
  type: 'line',
  data: data2,
  options: {
    interaction: {
      intersect: false,
      mode: 'x',
    },
  }
};



const myChart1 = new Chart(
  document.getElementById('myChart1').getContext("2d"),
  config1
);
const myChart2 = new Chart(
  document.getElementById('myChart2').getContext("2d"),
  config2
);



