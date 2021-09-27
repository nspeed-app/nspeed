package webview

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/templates"
	"github.com/rs/cors"

	"github.com/go-echarts/statsview/statics"
	"github.com/go-echarts/statsview/viewer"
)

// ViewManager
type ViewManager struct {
	srv *http.Server

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
func New() *ViewManager {
	page := components.NewPage()
	page.PageTitle = "NSpeed"
	page.AssetsHost = fmt.Sprintf("http://%s/debug/statsview/statics/", viewer.LinkAddr())
	page.Assets.JSAssets.Add("jquery.min.js")
	page.Assets.JSAssets.Add("nspeed.js")

	mgr := &ViewManager{
		srv: &http.Server{
			Addr:           viewer.Addr(),
			ReadTimeout:    time.Minute,
			WriteTimeout:   time.Minute,
			MaxHeaderBytes: 1 << 20,
		},
	}
	mgr.Ctx, mgr.Cancel = context.WithCancel(context.Background())
	mgr.Register(
		//NewCPUViewer(),
		NewNetInterfacesViewer("rx"),
		//NewNetInterfacesViewer("tx"),
	)
	// todo: put these in a new tab
	// mgr.PerfRegister(
	// 	viewer.NewGoroutinesViewer(),
	// 	viewer.NewHeapViewer(),
	// 	viewer.NewStackViewer(),
	// 	viewer.NewGCNumViewer(),
	// 	viewer.NewGCSizeViewer(),
	// 	viewer.NewGCCPUFractionViewer(),
	// )

	smgr := NewStatsMgr(mgr.Ctx)
	lsmgr := viewer.NewStatsMgr(mgr.Ctx)
	for _, v := range mgr.Views {
		v.SetStatsMgr(smgr)
	}
	for _, v := range mgr.PerfViews {
		v.SetStatsMgr(lsmgr)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	for _, v := range mgr.Views {
		page.AddCharts(v.View())
		mux.HandleFunc("/debug/statsview/view/"+v.Name(), v.Serve)
	}
	for _, v := range mgr.PerfViews {
		page.AddCharts(v.View())
		mux.HandleFunc("/debug/statsview/view/"+v.Name(), v.Serve)
	}

	staticsPath := "/debug/statsview/statics/"
	fstatic, err := fs.Sub(staticFiles, "statics")
	fs.WalkDir(fstatic, ".", func(path string, d fs.DirEntry, err error) error {
		fmt.Println(path)
		return nil
	})
	if err != nil {
		panic("statsview: failed to get embeded static filesystem: " + err.Error())
	}
	mux.Handle(staticsPath, http.StripPrefix(staticsPath, http.FileServer(http.FS(fstatic))))

	mux.HandleFunc("/debug/statsview", func(w http.ResponseWriter, _ *http.Request) {
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
	return mgr
}
