package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title: "채널 노트", Width: 1280, Height: 820, MinWidth: 900, MinHeight: 600,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 30, G: 31, B: 34, A: 1},
		OnStartup:        app.startup, OnBeforeClose: app.beforeClose,
		Bind:    []interface{}{app},
		Windows: &windows.Options{WebviewIsTransparent: false, WindowIsTranslucent: false},
	})
	if err != nil {
		log.Fatal(err)
	}
}
