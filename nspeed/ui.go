package nspeed

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"

	_ "embed"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"nspeed.app/metric"
	"nspeed.app/nspeed/ui"
	"nspeed.app/utils"
)

//go:embed embed/ui/*
var uiRoot embed.FS
var uiPrefix = "embed/ui" // keep in sync with the go:embed above

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func uiRootHandler() http.Handler {
	sub, err := fs.Sub(uiRoot, uiPrefix)
	if err != nil {
		panic(err)
	}
	return http.FileServerFS(sub)
}

type UIHost struct {
	JobData
	Parameters    *HostParameters
	Server        *JobHTTPServer // underlying endpoint
	OpenBrowser   bool
	globalOptions Options
	hub           *ui.Hub
}

func (j *UIHost) GetTarget() string {
	return fmt.Sprintf("server %s:%d (IP version: %d)", j.Parameters.Host, j.Parameters.Port, j.Parameters.IPVersion)
}

func (j *UIHost) GoString() string {
	if zerolog.GlobalLevel() > zerolog.InfoLevel {
		return fmt.Sprintf("UI server port: %d", j.Parameters.Port)
	}
	return fmt.Sprintf("UI {Address: %q, port: %d, IPversion: %d, UseTLS: %t}",
		j.Parameters.Host,
		j.Parameters.Port,
		j.Parameters.IPVersion,
		j.Parameters.ServeTLS,
	)
}

func (j *UIHost) PrintString(_, _ bool) string {
	return "UI " + net.JoinHostPort(j.Parameters.Host, strconv.Itoa(j.Parameters.Port))
}

func (j *UIHost) IsInfinite() bool {
	return true
}

func (j *UIHost) Init(opt Options) error {

	j.globalOptions = opt

	j.Parameters.IPVersion = utils.OrDefault(j.Parameters.IPVersion, opt.IPVersion)

	if j.Parameters == nil {
		j.Parameters = NewHostParameters()
		j.Parameters.APIOnly = true
	}
	if j.Server == nil {
		j.Parameters.APIOnly = true
		j.Server = &JobHTTPServer{Parameters: j.Parameters}
		err := j.Server.Init(opt)
		if err != nil {
			return err
		}
	}
	if j.Server.Mux == nil {
		return errors.New("UI: no Mux in server")
	}

	j.Server.Mux.Handle("/", uiRootHandler())
	j.Server.Mux.Handle("/ws", http.HandlerFunc(j.uiHandler))

	return nil
}

func (j *UIHost) Run(ctx context.Context, wg *sync.WaitGroup, opt Options) (*metric.Metric, error) {
	if j.Server == nil {
		return nil, errors.New("ui: no server")
	}
	j.Server.RunHook = func(c context.Context) error {
		return j.PostRun()
	}
	// start the hub
	if j.hub != nil {
		return nil, fmt.Errorf("ui hub already exists!")
	}
	j.hub = ui.NewHub()
	ctx, cancel := context.WithCancel(ctx)
	go j.hub.Run(ctx, cancel)
	return j.Server.Run(ctx, wg, opt)
}

// PostRun is called when the endpoint server is running
func (j *UIHost) PostRun() error {
	url := j.Server.URL
	if j.OpenBrowser {
		j.Logger().Warn().Msgf("opening browser at %s", url)
		return utils.OpenBrowser(url.String(), nil, nil)
	}
	return nil
}

func (j *UIHost) uiHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		log.Println(err)
		return
	}
	client := &ui.Client{Hub: j.hub, Conn: conn, Send: make(chan []byte, 256)}
	client.Hub.Register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.WritePump()
	go client.ReadPump()
}

func CmdUIParse(command string, cmdargs []string, jl *JobList, output io.Writer) ([]string, error) {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(output)
	host := NewHostParameters()
	j := UIHost{Parameters: host}

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage of %s: ", uiCommand)
		fmt.Fprintf(fs.Output(), "  %s [options]\n", uiCommand)
		fmt.Fprintf(fs.Output(), "\nAvailable options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(fs.Output(), "\n")
	}

	fs.StringVar(&host.Host, "a", "localhost", "Hostname or ip address to listen (use \"\" for all interfaces/all addresses)")
	//flag.Lookup("address").NoOptDefVal = ""
	useIPv4 := fs.Bool("4", false, "use IPv4 only")
	useIPv6 := fs.Bool("6", false, "use IPv6 only")
	fs.IntVar(&host.Port, "p", 0, "Port number to listen on")
	fs.StringVar(&host.TLSCertFile, "cert", "", "TLS certificate file")
	fs.StringVar(&host.TLSKeyFile, "key", "", "TLS Key file")
	fs.BoolVar(&host.UseTLSSelfConfig, "self", false, "listen in HTTPS using a self-signed TLS certificate")
	noBrowser := fs.Bool("nobrowse", false, "do not open browser")

	if err := fs.Parse(cmdargs); err != nil {
		return cmdargs, err
	}

	// force IPv4 or IPv6
	if *useIPv4 && *useIPv6 {
		fs.Usage()
		return cmdargs, errors.New("specify IPv4 or IPv6 but not both")
	}

	if *useIPv4 {
		host.IPVersion = 4
	}
	if *useIPv6 {
		host.IPVersion = 6
	}
	j.OpenBrowser = !*noBrowser

	args := fs.Args()
	err := jl.AddJobs(&j)
	return args, err
}
