module nspeed.app

go 1.18

require (
	github.com/go-echarts/go-echarts/v2 v2.2.4
	github.com/go-echarts/statsview v0.3.4
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58
	github.com/rs/zerolog v1.26.1
	github.com/shirou/gopsutil/v3 v3.22.3
	golang.org/x/exp v0.0.0-20220426173459-3bcf042a4bf5
	golang.org/x/net v0.0.0-20220425223048-2871e0cb64e4
	golang.org/x/sys v0.0.0-20220422013727-9388b58f7150
	golang.zx2c4.com/wireguard/windows v0.5.3
)

require (
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/tklauser/go-sysconf v0.3.10 // indirect
	github.com/tklauser/numcpus v0.4.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	golang.org/x/text v0.3.8-0.20211105212822-18b340fc7af2 // indirect
)

// replace golang.org/x/net => ../golang-net
