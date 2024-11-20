#!bin/sh
# update the versions in this file then execute it
# https://github.com/chartjs/Chart.js
curl -o chart.js "https://cdn.jsdelivr.net/npm/chart.js@4.4.6/dist/chart.umd.min.js"
# https://github.com/date-fns/date-fns
curl -o date-fns.js "https://cdnjs.cloudflare.com/ajax/libs/date-fns/1.30.1/date_fns.js"
# https://github.com/chartjs/chartjs-adapter-date-fns
curl -o chartjs-adapter-date-fns.js "https://cdn.jsdelivr.net/npm/chartjs-adapter-date-fns/dist/chartjs-adapter-date-fns.bundle.min.js"