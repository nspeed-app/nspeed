module nspeed.app

go 1.18

require (
	github.com/go-echarts/go-echarts/v2 v2.2.4
	github.com/go-echarts/statsview v0.3.4
	github.com/lucas-clemente/quic-go v0.27.1
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58
	github.com/pkg/browser v0.0.0-20210911075715-681adbf594b8
	github.com/rs/zerolog v1.26.1
	github.com/shirou/gopsutil/v3 v3.22.5
	golang.org/x/exp v0.0.0-20220602145555-4a0574d9293f
	golang.org/x/net v0.0.0-20220531201128-c960675eff93
	golang.org/x/sys v0.0.0-20220520151302-bc2c85ada10a
	golang.zx2c4.com/wireguard/windows v0.5.3
)

require (
	github.com/cheekybits/genny v1.0.0 // indirect
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-task/slim-sprig v0.0.0-20210107165309-348f09dbbbc0 // indirect
	github.com/lufia/plan9stats v0.0.0-20220517141722-cf486979b281 // indirect
	github.com/marten-seemann/qpack v0.2.1 // indirect
	github.com/marten-seemann/qtls-go1-16 v0.1.5 // indirect
	github.com/marten-seemann/qtls-go1-17 v0.1.1 // indirect
	github.com/marten-seemann/qtls-go1-18 v0.1.1 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/power-devops/perfstat v0.0.0-20220216144756-c35f1ee13d7c // indirect
	github.com/tklauser/go-sysconf v0.3.10 // indirect
	github.com/tklauser/numcpus v0.5.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	golang.org/x/crypto v0.0.0-20220525230936-793ad666bf5e // indirect
	golang.org/x/mod v0.6.0-dev.0.20220106191415-9b9b3d81d5e3 // indirect
	golang.org/x/text v0.3.8-0.20211105212822-18b340fc7af2 // indirect
	golang.org/x/tools v0.1.10 // indirect
	golang.org/x/xerrors v0.0.0-20220517211312-f3a8303e98df // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
)

replace golang.org/x/net => ../golang-net
