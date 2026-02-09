package nspeed

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"nspeed/metrics"

	"nspeed/nspeed/ui"
	"nspeed/nspeed/ui/goapp"

	"nspeed/webview"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"nspeed.app/nspeed/generics"
	"nspeed.app/nspeed/str"
	"nspeed.app/nspeed/web"
)

//go:embed embed/ui/app.wasm
var goappwasm []byte

//go:embed embed/ui/*
var uiRoot embed.FS
var uiPrefix = "embed/ui" // keep in sync with the go:embed above

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type UIHost struct {
	JobData
	Parameters    *HostParameters
	Server        *JobHTTPServer // underlying endpoint
	OpenBrowser   bool
	globalOptions Options
	hub           *ui.Hub
	viewerMode    bool
	goapp         bool
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

	j.Parameters.IPVersion = generics.OrDefault(j.Parameters.IPVersion, opt.IPVersion)

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

	if j.viewerMode {
		j.Server.Mux.Handle("/", j)
		return nil
	}

	j.Server.Mux.Handle("/ws", http.HandlerFunc(j.uiHandler))

	if j.goapp {
		//go-app requires "/web", can't be changed
		j.Server.Mux.Handle("/web/", http.StripPrefix("/web/", j.uiRootHandler()))
		j.Server.Mux.Handle("/", &goapp.RootHandler)
	}

	if !j.goapp {
		j.Server.Mux.Handle("/", j.uiRootHandler())
	}

	return nil
}

func (j *UIHost) Run(ctx context.Context, wg *sync.WaitGroup, opt Options) (*metrics.Metric, error) {

	if j.Server == nil {
		return nil, errors.New("ui: no server")
	}
	j.Server.RunHook = func(c context.Context) error {
		return j.PostRun()
	}
	if j.goapp {
		goapp.Init()
	}
	if !j.viewerMode {
		// start the hub
		if j.hub != nil {
			return nil, fmt.Errorf("ui hub already exists")
		}
		j.hub = ui.NewHub()
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		j.hub.Execute = func(args string) {
			err := j.RunCmd(ctx, str.Fields(args))
			if err != nil {
				j.Logger().Err(err).Msgf("Execute")
			}
		}
		logger := j.Logger().Output(j.hub)
		j.SetLogger(&logger)
		go j.hub.Run(ctx, cancel)
	}
	return j.Server.Run(ctx, wg, opt)
}

func (f *UIHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if f.goapp {
		fmt.Println("serving app.wasm")
		http.ServeContent(w, r, "app.wasm", time.Now(), bytes.NewReader(goappwasm))
		return
	}

	html, err := web.InlineFromString(webview.ViewerTmplHTML, webview.StaticFiles, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, "viewer.html", time.Now(), bytes.NewReader([]byte(html)))
}

// func debugMiddleware(next http.Handler) http.Handler {
// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		fmt.Println("http request:", r.URL.Path)
// 		next.ServeHTTP(w, r)
// 	})
// }

func (j *UIHost) uiRootHandler() http.Handler {
	// normal mode
	sub, err := fs.Sub(uiRoot, uiPrefix)
	if err != nil {
		panic(err)
	}
	return http.FileServerFS(sub)
}

// PostRun is called when the endpoint server is running
func (j *UIHost) PostRun() error {
	url := j.Server.URL
	if j.OpenBrowser {
		j.Logger().Warn().Msgf("opening browser at %s", url)
		return web.OpenBrowser(url.String(), nil, nil)
	}
	return nil
}

func (j *UIHost) uiHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		log.Error().Err(err).Msg("ws upgrade error")
		return
	}
	client := j.hub.Subscribe(conn)

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go func() {
		err := client.WritePump()
		if err != nil {
			j.logger.Error().Err(err).Msgf("writepump error")
		}
	}()
	go func() {
		err := client.ReadPump()
		if err != nil {
			j.logger.Error().Err(err).Msgf("writepump error")
		}
	}()
	j.hub.SendMessage([]byte("server version = " + ServerVersion))
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
	viewer := fs.Bool("viewer", false, "viewer mode: a page for viewing json result files")
	goapp := fs.Bool("goapp", true, "use go-app")
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
	j.viewerMode = *viewer
	j.goapp = *goapp

	args := fs.Args()
	err := jl.AddJobs(&j)
	return args, err
}

func (j *UIHost) RunCmd(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("RunCmd no args")
	}
	cmd := args[0]
	args = args[1:]

	marshaller := func(v any) ([]byte, error) {
		return json.MarshalIndent(v, "", "\t")
	}

	results, err := APIRun(ctx, j.globalOptions, cmd, args, j, "ui", marshaller)
	if err != nil {
		return err
	}
	j.hub.SendMessage(results)
	return nil
}

type logmsg struct {
	Message string `json:"message"`
}

func (j *UIHost) Write(msg []byte) (int, error) {
	js, err := json.Marshal(&logmsg{Message: string(msg)})
	if err != nil {
		return 0, err
	}
	j.hub.SendMessage(js)
	return len(msg), nil
}
