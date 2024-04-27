module nspeed.app

go 1.21.0

require (
	github.com/arl/statsviz v0.6.0
	github.com/go-echarts/go-echarts v1.0.0
	github.com/go-echarts/go-echarts/v2 v2.3.3
	github.com/go-echarts/statsview v0.3.4
	github.com/google/gopacket v1.1.19
	github.com/gorilla/websocket v1.5.1
	github.com/libp2p/go-netroute v0.2.1
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58
	github.com/quic-go/quic-go v0.43.0
	github.com/rs/xid v1.5.0
	github.com/rs/zerolog v1.32.0
	github.com/shirou/gopsutil/v3 v3.24.3
	golang.org/x/exp v0.0.0-20240416160154-fe59bbe5cc7f
	golang.org/x/net v0.24.0
	golang.org/x/sys v0.19.0
	golang.zx2c4.com/wireguard/windows v0.5.3
	nspeed.app/nspeed v0.11.0
)

require (
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/google/pprof v0.0.0-20240424215950-a892ee059fd6 // indirect
	github.com/lufia/plan9stats v0.0.0-20240408141607-282e7b5d6b74 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/onsi/ginkgo/v2 v2.17.1 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/quic-go/qpack v0.4.0 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/tklauser/go-sysconf v0.3.14 // indirect
	github.com/tklauser/numcpus v0.8.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.uber.org/mock v0.4.0 // indirect
	golang.org/x/crypto v0.22.0 // indirect
	golang.org/x/mod v0.17.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/tools v0.20.0 // indirect
)

//replace golang.org/x/net => ../golang-net
//replace github.com/quic-go/quic-go => ../quic-go
