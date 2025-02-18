module nspeed.app

go 1.24.0

require (
	github.com/arl/statsviz v0.6.0
	github.com/go-echarts/go-echarts/v2 v2.5.1
	github.com/go-echarts/statsview v0.4.2
	github.com/google/gopacket v1.1.19
	github.com/gorilla/websocket v1.5.3
	github.com/libp2p/go-netroute v0.2.2
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58
	github.com/quic-go/quic-go v0.49.0
	github.com/rs/xid v1.6.0
	github.com/rs/zerolog v1.33.0
	github.com/shirou/gopsutil/v4 v4.25.1
	golang.org/x/exp v0.0.0-20250215185904-eff6e970281f
	golang.org/x/net v0.35.0
	golang.org/x/sys v0.30.0
	golang.org/x/term v0.29.0
	golang.zx2c4.com/wireguard/windows v0.5.3
	nspeed.app/nspeed v0.12.0
)

require (
	github.com/ebitengine/purego v0.8.2 // indirect
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/google/pprof v0.0.0-20250208200701-d0013a598941 // indirect
	github.com/lufia/plan9stats v0.0.0-20240909124753-873cd0166683 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/onsi/ginkgo/v2 v2.22.2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/quic-go/qpack v0.5.1 // indirect
	github.com/tklauser/go-sysconf v0.3.14 // indirect
	github.com/tklauser/numcpus v0.9.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.uber.org/mock v0.5.0 // indirect
	golang.org/x/crypto v0.33.0 // indirect
	golang.org/x/mod v0.23.0 // indirect
	golang.org/x/sync v0.11.0 // indirect
	golang.org/x/text v0.22.0 // indirect
	golang.org/x/tools v0.30.0 // indirect
)

//replace golang.org/x/net => ../golang-net
//replace github.com/quic-go/quic-go => ../quic-go
