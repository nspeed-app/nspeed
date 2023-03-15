module nspeed.app

go 1.20

require (
	github.com/arl/statsviz v0.5.1
	github.com/go-echarts/go-echarts v1.0.0
	github.com/go-echarts/go-echarts/v2 v2.2.5
	github.com/go-echarts/statsview v0.3.4
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58
	github.com/quic-go/quic-go v0.33.0
	github.com/rs/xid v1.4.0
	github.com/rs/zerolog v1.29.0
	github.com/shirou/gopsutil/v3 v3.23.2
	golang.org/x/exp v0.0.0-20230315142452-642cacee5cc0
	golang.org/x/net v0.8.0
	golang.org/x/sys v0.6.0
	golang.zx2c4.com/wireguard/windows v0.5.3
)

require (
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-task/slim-sprig v0.0.0-20210107165309-348f09dbbbc0 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/google/pprof v0.0.0-20230309165930-d61513b1440d // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20230110061619-bbe2e5e100de // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.17 // indirect
	github.com/onsi/ginkgo/v2 v2.9.1 // indirect
	github.com/power-devops/perfstat v0.0.0-20221212215047-62379fc7944b // indirect
	github.com/quic-go/qpack v0.4.0 // indirect
	github.com/quic-go/qtls-go1-19 v0.2.1 // indirect
	github.com/quic-go/qtls-go1-20 v0.1.1 // indirect
	github.com/tklauser/go-sysconf v0.3.11 // indirect
	github.com/tklauser/numcpus v0.6.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	golang.org/x/crypto v0.7.0 // indirect
	golang.org/x/mod v0.9.0 // indirect
	golang.org/x/text v0.8.0 // indirect
	golang.org/x/tools v0.7.0 // indirect
)

//replace golang.org/x/net => ../golang-net
//replace github.com/lucas-clemente/quic-go => ../quic-go
