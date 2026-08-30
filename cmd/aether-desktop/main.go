package main

import (
	"log"

	aetherapp "github.com/eust-w/agentic-embedded-lab/internal/app"
	"github.com/eust-w/agentic-embedded-lab/internal/webassets"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

func main() {
	backend := aetherapp.NewBackend()
	err := wails.Run(&options.App{
		Title:            "Aether Desktop",
		Width:            1600,
		Height:           1000,
		MinWidth:         1180,
		MinHeight:        720,
		Frameless:        false,
		BackgroundColour: &options.RGBA{R: 11, G: 15, B: 20, A: 1},
		AssetServer:      &assetserver.Options{Assets: webassets.Assets},
		OnStartup:        backend.Startup,
		OnShutdown:       backend.Shutdown,
		Bind:             []any{backend},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			Appearance:           mac.NSAppearanceNameDarkAqua,
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
