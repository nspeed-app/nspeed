package goapp

import "github.com/maxence-charriere/go-app/v10/pkg/app"

// The main function is the entry point where the app is configured and started.
// It is executed in 2 different environments: A client (the web browser) and a
// server.
func Init() {
	app.Route("/", func() app.Composer { return &hello{} })
	app.Route("/info", func() app.Composer { return &info{} })
	app.RunWhenOnBrowser()
}
