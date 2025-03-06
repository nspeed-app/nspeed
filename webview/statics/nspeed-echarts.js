// helper functions for charting

// humanize bytes using std power of 10 prefix
function formatBytes(value, unitname = '', precision = 2) {
	if (value === 0) return '0 ' + unitname;
	if (value < 1) return value.toFixed(precision)
	const k = 1000;
	const dm = precision;
	const sizes = ['', 'k', 'M', 'G', 'T', 'P', 'E', 'Z', 'Y'];
	const i = Math.floor(Math.log(value) / Math.log(k));
	return parseFloat((value / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i] + unitname;
}

// format tooltip for bps 
function formatTooltip(params) {
	if (params === undefined || params.length == 0) return "NO DATA";

	let output = '<b>' + params[0].axisValueLabel + '</b><br/>'
	
	params.sort((a, b) => b.value[1] - a.value[1]);

	for (let i = 0; i < params.length; i++) {
		output += params[i].marker + params[i].seriesName + ': ' + formatBytes(params[i].value[1], "bps"); //
		if (i != params.length - 1) { // Append a <br/> tag if not last in loop
			output += '<br/>'
		}
	}
	return output
}
