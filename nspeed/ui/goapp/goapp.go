package goapp

import (
	"fmt"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

// hello is a component that displays a simple "Hello World!". A component is a
// customizable, independent, and reusable UI element. It is created by
// embedding app.Compo into a struct.
type hello struct {
	app.Compo
}

// The Render method is where the component appearance is defined. Here, a
// "Hello World!" is displayed as a heading.
func (h *hello) Render() app.UI {
	if app.IsClient {
		fmt.Println("HELLO APP UI I'm the client")
	}
	if app.IsServer {
		fmt.Println("HELLO APP UI I'm the server")
	}
	h1 := app.H1().Text("NSPEED LIVES!")
	return h1
}

type info struct {
	app.Compo
}

func (i *info) Render() app.UI {
	return app.H1().Text("INFO")
}

var RootHandler = app.Handler{
	Name:        "NSPEED",
	Description: "nspeed v0.0.0",
	Icon: app.Icon{
		Default: "/web/favicon.ico",
		SVG:     "/web/icon.svg", // Specify default favicon.
	},
	// Styles: []string{
	// 	"/web/hello.css", // Loads hello.css file.
	// },
	// Scripts: []string{
	// 	"/web/hello.js", // Loads hello.js file.
	// },
}
