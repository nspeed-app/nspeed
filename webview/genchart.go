package webview

import (
	"bytes"
	"context"
	_ "embed"
	"io/fs"
	"text/template"

	"github.com/rs/zerolog/log"
	"nspeed.app/utils"
)

//go:embed statics/viewer-echarts.html
var ViewerTmplHTML string // the template has only one unnamed action ({{.}}) which will be replaced with json data

var viewerTmpl *template.Template
var StaticFiles fs.FS

// review: panic in init() ?
func init() {
	var err error
	StaticFiles, err = fs.Sub(staticFiles, "statics")
	if err != nil {
		panic("genchart: failed to get embeded static filesystem: " + err.Error())
	}

	viewerTmpl, err = template.New("viewer").Parse(ViewerTmplHTML)
	if err != nil {
		panic("viewer.html: failed to parse as HTML template: " + err.Error())
	}
}

// GenerateHTMLReport produces HTML file from jsData
// if filename is ":" opens result from a temporary local web browser
func GenerateHTMLReport(ctx context.Context, htmlFilename string, jsData string) {
	var out *utils.SmartFile
	var err error
	if htmlFilename != ":" {
		out, err = utils.FileCreate(htmlFilename)
		if err != nil {
			log.Error().Msgf("could not create html file (%s): %v", htmlFilename, err)
			return
		}
	}
	// inject json in html template
	var htmlBuff bytes.Buffer
	err = viewerTmpl.ExecuteTemplate(&htmlBuff, "viewer", jsData)
	if err != nil {
		log.Error().Msgf("could not inject json into html file (%s): %v", htmlFilename, err)
		return
	}
	// output to file or directly open a browser
	if out != nil {
		html, err := utils.InlineAssets(htmlBuff.String(), StaticFiles, true)
		if err != nil {
			log.Fatal().Msgf("could inline html file (%s): %v", htmlFilename, err)
		}
		_, err = out.WriteString(html)
		if err != nil {
			log.Fatal().Msgf("could not write to html file (%s): %v", htmlFilename, err)
		}
		log.Info().Msgf("html generated to %s", htmlFilename)
	} else {
		html, err := utils.InlineAssets(htmlBuff.String(), StaticFiles, false)
		if err != nil {
			log.Fatal().Msgf("could inline html file (%s): %v", htmlFilename, err)
		}
		err = utils.OpenHTML(ctx, html, true)
		if err != nil {
			log.Fatal().Msgf("could not open html: %v", err)
		}
	}
}

func OpenViewer(ctx context.Context, openBrowser bool) {
	html, err := utils.InlineAssets(ViewerTmplHTML, StaticFiles, true)
	if err != nil {
		log.Fatal().Msgf("open embed viewer file: %v", err)
	}
	err = utils.OpenHTML(ctx, html, openBrowser)
	if err != nil {
		log.Fatal().Msgf("could not open html: %v", err)
	}
}
