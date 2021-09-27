function formatBytes(value, unitname = '', precision=2) {
	if (value === 0) return '0 Bytes';
	const k = 1000;
	const dm = precision;
	const sizes = ['', 'k', 'M', 'G', 'T', 'P', 'E', 'Z', 'Y'];
	const i = Math.floor(Math.log(value) / Math.log(k));
	return parseFloat((value / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i]+unitname;
}