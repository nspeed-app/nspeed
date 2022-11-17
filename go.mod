module nspeed.app

go 1.19

require (
	github.com/arl/statsviz v0.5.1
	github.com/go-echarts/go-echarts/v2 v2.2.4
	github.com/go-echarts/statsview v0.3.4
	github.com/lucas-clemente/quic-go v0.31.0
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58
	github.com/pkg/browser v0.0.0-20210911075715-681adbf594b8
	github.com/rs/zerolog v1.28.0
	github.com/shirou/gopsutil/v3 v3.22.10
	golang.org/x/exp v0.0.0-20221114191408-850992195362
	golang.org/x/net v0.2.0
	golang.org/x/sys v0.2.0
	golang.zx2c4.com/wireguard/windows v0.5.3
)

require (
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-task/slim-sprig v0.0.0-20210107165309-348f09dbbbc0 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/google/pprof v0.0.0-20221112000123-84eb7ad69597 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20220913051719-115f729f3c8c // indirect
	github.com/marten-seemann/qpack v0.3.0 // indirect
	github.com/marten-seemann/qtls-go1-18 v0.1.3 // indirect
	github.com/marten-seemann/qtls-go1-19 v0.1.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.16 // indirect
	github.com/onsi/ginkgo/v2 v2.5.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20220216144756-c35f1ee13d7c // indirect
	github.com/tklauser/go-sysconf v0.3.11 // indirect
	github.com/tklauser/numcpus v0.6.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	golang.org/x/crypto v0.3.0 // indirect
	golang.org/x/mod v0.7.0 // indirect
	golang.org/x/text v0.4.0 // indirect
	golang.org/x/tools v0.3.0 // indirect
)

//replace golang.org/x/net => ../golang-net
