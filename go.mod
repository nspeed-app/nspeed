module nspeed.app

go 1.17

require (
	github.com/go-echarts/go-echarts/v2 v2.2.4
	github.com/go-echarts/statsview v0.3.4
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58
	github.com/rs/zerolog v1.25.0
	github.com/shirou/gopsutil/v3 v3.21.9
	golang.org/x/net v0.0.0-20211020060615-d418f374d309
	golang.org/x/sys v0.0.0-20211216021012-1d35b9e2eb4e
	golang.zx2c4.com/wireguard/windows v0.5.1
)

require (
	github.com/StackExchange/wmi v1.2.1 // indirect
	github.com/go-ole/go-ole v1.2.5 // indirect
	github.com/tklauser/go-sysconf v0.3.9 // indirect
	github.com/tklauser/numcpus v0.3.0 // indirect
	golang.org/x/text v0.3.8-0.20211004125949-5bd84dd9b33b // indirect
)

// replace golang.org/x/net => ../golang-net
