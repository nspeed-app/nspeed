package nspeed

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	_ "embed"

	"github.com/rs/zerolog"
	"nspeed.app/metric"
	"nspeed.app/utils"
	"nspeed.app/webview"
)

const APIVersion string = "1.0"
const StatsPath = "/rt"

//go:embed API.md
var APIDocumentation []byte

type APIHost struct {
	JobData
	Parameters        *HostParameters
	Server            *JobHTTPServer // underlying endpoint
	WebStats          bool
	WebStatsOnly      bool
	WebStatsDebug     bool
	OpenBrowser       bool
	WebStatsInterface []string
	globalOptions     Options
}

func (j *APIHost) GetTarget() string {
	return fmt.Sprintf("server %s:%d (IP version: %d)", j.Parameters.Host, j.Parameters.Port, j.Parameters.IPVersion)
}

func (j *APIHost) GoString() string {
	if zerolog.GlobalLevel() > zerolog.InfoLevel {
		return fmt.Sprintf("API server port: %d", j.Parameters.Port)
	}
	return fmt.Sprintf("API {Address: %q, port: %d, IPversion: %d, UseTLS: %t}",
		j.Parameters.Host,
		j.Parameters.Port,
		j.Parameters.IPVersion,
		j.Parameters.ServeTLS,
	)
}

func (j *APIHost) PrintString(_, _ bool) string {
	return "API " + net.JoinHostPort(j.Parameters.Host, strconv.Itoa(j.Parameters.Port))
}

func (j *APIHost) IsInfinite() bool {
	return true
}

func (j *APIHost) Init(opt Options) error {

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
		return errors.New("API: no Mux in server")
	}
	if !j.WebStatsOnly {
		j.Server.Mux.Handle("/api/v1/", http.StripPrefix("/api/v1/", http.HandlerFunc(j.apiHandler)))
		j.Server.Mux.Handle("/api/", http.HandlerFunc(j.apiHelpHandler))
	}
	if j.WebStats {
		mgr, err := webview.NewViewManager(opt.ProgressInterval, StatsPath, j.WebStatsInterface, j.WebStatsDebug)
		if err != nil {
			return err
		}
		mgr.RegisterRoutes(j.Server.Mux)
	}
	return nil
}

func (j *APIHost) Run(ctx context.Context, wg *sync.WaitGroup, opt Options) (*metric.Metric, error) {
	if j.Server == nil {
		return nil, errors.New("API: no server")
	}
	j.Server.RunHook = func(c context.Context) error {
		return j.PostRun()
	}
	return j.Server.Run(ctx, wg, opt)
}

func (j *APIHost) apiHelpHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write(APIDocumentation)
}

func (j *APIHost) apiHandler(w http.ResponseWriter, r *http.Request) {

	method := r.Method
	if method == "" {
		method = "GET"
	}
	path := r.URL.Path
	j.Logger().Warn().Msgf("apiHandler: %s - %s from %s", method, path, r.RemoteAddr)

	if method != "GET" {
		http.Error(w, "no such method", http.StatusNotImplemented)
		return
	}

	// parse the URL path, cmd is first word then everywthing after the first '/' is an array of parameters
	// "cmd/param1/param1/param1"
	args := strings.Split(path, "/")
	cmd := args[0]
	args = args[1:]

	// http api specific
	if cmd == "headers" {
		for name, headers := range r.Header {
			for _, h := range headers {
				fmt.Fprintf(w, "%v: %v\n", name, h)
			}
		}
		fmt.Fprint(w, "\n")
		return
	}
	if cmd == "ip" {
		fmt.Fprintf(w, "%s\n", r.RemoteAddr)
		return //todo: add geoip query https://github.com/oschwald/maxminddb-golang
	}

	a, ok := r.URL.Query()["args"]
	if ok {
		for i := range a {
			args = append(args, strings.Fields(a[i])...)
		}
	}
	results, err := APIRun(r.Context(), j.globalOptions, cmd, args, w, r.RemoteAddr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		j.logger.Error().Err(err).Msgf("APIRun error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(results)
}

// PostRun is called when the endpoint server is running
func (j *APIHost) PostRun() error {
	url := j.Server.URL.JoinPath(StatsPath)
	if j.WebStats {
		j.Logger().Warn().Msgf("real time web stats at %s", url)
	}
	if j.WebStatsDebug {
		// we should get the route from wher it's defined
		// add a link in rt to this ?
		j.Logger().Warn().Msgf("Go runtime stats at %s", j.Server.URL.JoinPath(webview.GoStatsRoute))
		if !j.WebStats {
			url = j.Server.URL.JoinPath(webview.GoStatsRoute)
		}
	}
	if j.OpenBrowser {
		j.Logger().Warn().Msgf("opening browser at %s", url)
		return utils.OpenBrowser(url.String(), nil, nil)
	}
	return nil
}

// todo: merge server and api commands ?
func CmdAPIParse(command string, cmdargs []string, jl *JobList, output io.Writer) ([]string, error) {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(output)
	host := NewHostParameters()
	j := APIHost{Parameters: host}

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage of %s: ", apiCommand)
		fmt.Fprintf(fs.Output(), "  %s [options]\n", apiCommand)
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
	fs.BoolVar(&j.WebStats, "stats", false, "enable web stats at /rt url")
	fs.BoolVar(&j.WebStatsOnly, "statsonly", false, "enable web stats only (implies -stats)")
	fs.BoolVar(&j.WebStatsDebug, "statsdebug", false, "enable web stats debug (GO specifics info, implies -stats)")
	fs.BoolVar(&j.OpenBrowser, "browse", false, "open browser (implies -stats) - work for localhost only for now (wip)")
	interfaces := fs.String("statsinterfaces", "", "network interfaces to include (comma separated - default : all - implies -stats)")

	if err := fs.Parse(cmdargs); err != nil {
		return cmdargs, err
	}

	if j.WebStatsOnly || j.WebStatsDebug || j.OpenBrowser || *interfaces != "" {
		j.WebStats = true
	}

	if *interfaces != "" {
		j.WebStatsInterface = strings.Split(*interfaces, ",")
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

	args := fs.Args()
	err := jl.AddJobs(&j)
	return args, err
}
