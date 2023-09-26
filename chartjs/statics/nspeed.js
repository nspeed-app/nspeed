function formatBytes(value, unitname = '', precision = 2) {
	if (value === 0) return '0 ' + unitname;
	const k = 1000;
	const dm = precision;
	const sizes = ['', 'k', 'M', 'G', 'T', 'P', 'E', 'Z', 'Y'];
	const i = Math.floor(Math.log(value) / Math.log(k));
	return parseFloat((value / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i] + unitname;
}


function formatTooltip(params) {
	output = '<b>' + params[0].name + '</b><br/>'
	for (i = 0; i < params.length; i++) {
		output += params[i].marker + params[i].seriesName + ': ' + formatBytes(params[i].value, "bps"); //
		if (i != params.length - 1) { // Append a <br/> tag if not last in loop
			output += '<br/>'
		}
	}
	return output
}

function onLegendHover(event, item, legend) {
	console.log(event);
	console.log(item);
	console.log(legend);
	const activeElement = {
		datasetIndex: item.datasetIndex,
		index: 0,
	  };
	legend.chart.setActiveElements([activeElement]); // to also show thick border
	legend.chart.tooltip.setActiveElements([activeElement]);
	legend.chart.update();
}