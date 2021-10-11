package webview

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"

	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/templates"
	"github.com/rs/cors"

	"github.com/go-echarts/statsview/statics"
	"github.com/go-echarts/statsview/viewer"
)

// ViewManager
type ViewManager struct {
	srv   *http.Server
	path  string
	tick  time.Duration
	debug bool

	Smgr      *StatsMgr
	Ctx       context.Context
	Cancel    context.CancelFunc
	Views     []Viewer
	PerfViews []viewer.Viewer
}

// Register registers views to the ViewManager
func (vm *ViewManager) Register(views ...Viewer) {
	vm.Views = append(vm.Views, views...)
}

// PerfRegister registers perf views to the external ViewManager
func (vm *ViewManager) PerfRegister(views ...viewer.Viewer) {
	vm.PerfViews = append(vm.PerfViews, views...)
}

// Start runs a http server and begin to collect metrics
func (vm *ViewManager) Start() error {
	fmt.Printf("web stats wiew available at: http://%s/%s\n", vm.srv.Addr, vm.path) //http://localhost:18066/statsview")
	if vm.debug {
		fmt.Printf("pprof wiew available at: http://%s/debug/pprof\n", vm.srv.Addr) //localhost:18066/debug/pprof")
	}
	return vm.srv.ListenAndServe()
}

// Stop shutdown the http server gracefully
func (vm *ViewManager) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	vm.srv.Shutdown(ctx)
	vm.Cancel()
}

func init() {
	templates.PageTpl = `
{{- define "page" }}
<!DOCTYPE html>
<html>
    {{- template "header" . }}
<body>
<style> .box { justify-content:center; display:flex; flex-wrap:wrap } </style>
<div class="box"> {{- range .Charts }} {{ template "base" . }} {{- end }} </div>
</body>
</html>
{{ end }}
`
}

//go:embed statics
var staticFiles embed.FS

// New creates a new ViewManager instance
func NewViewManager(tick time.Duration, path string, debug bool) *ViewManager {

	path = strings.TrimSuffix(path, "/")
	path = strings.TrimPrefix(path, "/")

	page := components.NewPage()
	page.PageTitle = "NSpeed Webview"
	page.AssetsHost = fmt.Sprintf("http://%s/%s/statics/", viewer.LinkAddr(), path)
	page.Assets.JSAssets.Add("jquery.min.js")
	page.Assets.JSAssets.Add("nspeed.js")

	mgr := &ViewManager{
		srv: &http.Server{
			Addr:           viewer.Addr(),
			ReadTimeout:    time.Minute,
			WriteTimeout:   time.Minute,
			MaxHeaderBytes: 1 << 20,
		},
		tick:  tick,
		path:  path,
		debug: debug,
	}
	mgr.Ctx, mgr.Cancel = context.WithCancel(context.Background())
	//todo : may be add a message  if no support
	if HasNetStats() == nil {
		mgr.Register(
			NewNetInterfacesViewer(tick, "tx", mgr.path),
			NewNetInterfacesViewer(tick, "rx", mgr.path),
		)
	}
	mgr.Register(
		NewCPUViewer(tick, mgr.path),
	)
	if debug {
		mgr.PerfRegister(
			viewer.NewGoroutinesViewer(),
			viewer.NewHeapViewer(),
			viewer.NewStackViewer(),
			viewer.NewGCNumViewer(),
			viewer.NewGCSizeViewer(),
			viewer.NewGCCPUFractionViewer(),
		)
	}

	mux := http.NewServeMux()
	mgr.srv.Handler = mux

	smgr := NewStatsMgr(mgr.Ctx, tick)
	for _, v := range mgr.Views {
		v.SetStatsMgr(smgr)
	}
	if debug {
		lsmgr := viewer.NewStatsMgr(mgr.Ctx)
		for _, v := range mgr.PerfViews {
			v.SetStatsMgr(lsmgr)
		}
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}
	for _, v := range mgr.Views {
		page.AddCharts(v.View())
		fmt.Println("registering:" + v.Name())
		mux.HandleFunc("/"+path+"/"+v.Name(), v.Serve)
	}
	if debug {
		for _, v := range mgr.PerfViews {
			page.AddCharts(v.View())
			mux.HandleFunc("/"+path+v.Name(), v.Serve)
		}
	}

	staticsPath := "/" + path + "/statics/"
	fstatic, err := fs.Sub(staticFiles, "statics")
	// fs.WalkDir(fstatic, ".", func(path string, d fs.DirEntry, err error) error {
	// 	fmt.Println(path)
	// 	return nil
	// })
	if err != nil {
		panic("statsview: failed to get embeded static filesystem: " + err.Error())
	}
	mux.Handle(staticsPath, http.StripPrefix(staticsPath, http.FileServer(http.FS(fstatic))))

	mux.HandleFunc("/"+path, func(w http.ResponseWriter, r *http.Request) {
		page.Render(w)
	})

	_, err = http.FS(fstatic).Open("jquery.min.js")
	if err != nil {
		panic("statsview: failed to get embeded file: " + err.Error())
	}

	mux.HandleFunc(staticsPath+"themes/westeros.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(statics.WesterosJS))
	})

	mux.HandleFunc(staticsPath+"themes/macarons.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(statics.MacaronsJS))
	})

	mgr.srv.Handler = cors.AllowAll().Handler(mux)

	// v := reflect.ValueOf(mux).Elem()
	// spew.Dump(v.FieldByName("m").MapKeys())

	return mgr
}
