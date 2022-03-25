import {} from "../chart.min.js";


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

const footer = (tooltipItems) => {
  let sum = 0;

  tooltipItems.forEach(function(tooltipItem) {
    sum += tooltipItem.parsed.y;
  });
  return 'Sum: ' + sum;
};

const config1 = {
  type: 'line',
  data: data1,
  options: {
    interaction: {
      intersect: false,
      mode: 'index',
    },
    plugins: {
      tooltip: {
        callbacks: {
          footer: footer,
        }
      }
    }
  }
};

const config2 = {
  type: 'line',
  data: data2,
  options: {
    interaction: {
      intersect: false,
      mode: 'index',
    },
    plugins: {
      tooltip: {
        callbacks: {
          footer: footer,
        }
      }
    }
  }
};

const myChart1 = new Chart(
  document.getElementById('myChart1'),
  config1
);
const myChart2 = new Chart(
  document.getElementById('myChart2'),
  config2
);
