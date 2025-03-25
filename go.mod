module nspeed

go 1.24.1

require (
	github.com/arl/statsviz v0.6.0
	github.com/charmbracelet/lipgloss v1.1.0
	github.com/go-echarts/go-echarts/v2 v2.5.2
	github.com/go-echarts/statsview v0.4.2
	github.com/gorilla/websocket v1.5.3
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58
	github.com/quic-go/quic-go v0.50.1
	github.com/rs/xid v1.6.0
	github.com/rs/zerolog v1.34.0
	github.com/shirou/gopsutil/v4 v4.25.2
	go.opentelemetry.io/otel v1.35.0
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v1.35.0
	go.opentelemetry.io/otel/metric v1.35.0
	go.opentelemetry.io/otel/sdk v1.35.0
	go.opentelemetry.io/otel/sdk/metric v1.35.0
	golang.org/x/exp v0.0.0-20250305212735-054e65f0b394
	golang.org/x/net v0.37.0
	golang.org/x/sys v0.31.0
	golang.org/x/term v0.30.0
	golang.zx2c4.com/wireguard/windows v0.5.3
	nspeed.app v0.0.0-00010101000000-000000000000
	nspeed.app/nspeed v0.12.0
)

require (
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/charmbracelet/colorprofile v0.2.3-0.20250311203215-f60798e515dc // indirect
	github.com/charmbracelet/x/ansi v0.8.0 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.13 // indirect
	github.com/charmbracelet/x/exp/golden v0.0.0-20240815200342-61de596daa2b // indirect
	github.com/charmbracelet/x/term v0.2.1 // indirect
	github.com/ebitengine/purego v0.8.2 // indirect
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/google/gopacket v1.1.19 // indirect
	github.com/google/pprof v0.0.0-20250317173921-a4b03ec1a45e // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/libp2p/go-netroute v0.2.2 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20250303091104-876f3ea5145d // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/muesli/termenv v0.16.0 // indirect
	github.com/onsi/ginkgo/v2 v2.23.3 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/quic-go/qpack v0.5.1 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/tklauser/go-sysconf v0.3.15 // indirect
	github.com/tklauser/numcpus v0.10.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel/trace v1.35.0 // indirect
	go.uber.org/mock v0.5.0 // indirect
	golang.org/x/crypto v0.36.0 // indirect
	golang.org/x/mod v0.24.0 // indirect
	golang.org/x/sync v0.12.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	golang.org/x/tools v0.31.0 // indirect
)

//replace golang.org/x/net => ../golang-net
//replace github.com/quic-go/quic-go => ../quic-go
replace nspeed.app => ../nspeed
