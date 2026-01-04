module nspeed

go 1.25.5

//replace golang.org/x/net => ../golang-net
//replace github.com/quic-go/quic-go => ../quic-go

replace nspeed.app => ../nspeed

require (
	github.com/arl/statsviz v0.8.0
	github.com/charmbracelet/lipgloss v1.1.0
	github.com/go-echarts/go-echarts/v2 v2.6.7
	github.com/go-echarts/statsview v0.4.2
	github.com/gorilla/websocket v1.5.3
	github.com/magefile/mage v1.15.0
	github.com/maxence-charriere/go-app/v10 v10.1.8
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58
	github.com/quic-go/quic-go v0.58.0
	github.com/rs/xid v1.6.0
	github.com/rs/zerolog v1.34.0
	github.com/shirou/gopsutil/v4 v4.25.12
	github.com/stretchr/testify v1.11.1
	go.opentelemetry.io/otel v1.39.0
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v1.39.0
	go.opentelemetry.io/otel/metric v1.39.0
	go.opentelemetry.io/otel/sdk v1.39.0
	go.opentelemetry.io/otel/sdk/metric v1.39.0
	golang.org/x/exp v0.0.0-20251219203646-944ab1f22d93
	golang.org/x/net v0.48.0
	golang.org/x/sys v0.39.0
	golang.org/x/term v0.38.0
	golang.zx2c4.com/wireguard/windows v0.5.3
	nspeed.app v0.0.0-00010101000000-000000000000
)

require (
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/charmbracelet/colorprofile v0.4.1 // indirect
	github.com/charmbracelet/x/ansi v0.11.3 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.14 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.6.2 // indirect
	github.com/clipperhouse/stringish v0.1.1 // indirect
	github.com/clipperhouse/uax29/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/ebitengine/purego v0.9.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/google/gopacket v1.1.19 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/libp2p/go-netroute v0.4.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.3.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20251013123823-9fd1530e3ec3 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.19 // indirect
	github.com/muesli/termenv v0.16.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/trace v1.39.0 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
