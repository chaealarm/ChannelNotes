package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed default-icon.png
var defaultIcon []byte

func main() {
	service := NewApp()
	wailsApp := application.New(application.Options{
		Name: "ChannelNotes", Description: "채널과 그룹으로 정리하는 데스크톱 메모장",
		Services:   []application.Service{application.NewService(service)},
		Assets:     application.AssetOptions{Handler: application.BundledAssetFileServer(assets)},
		OnShutdown: service.shutdown,
	})
	service.wails = wailsApp
	mainWindow := wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "main", Title: "Channel Notes", Width: 1280, Height: 820, MinWidth: 900, MinHeight: 600, Frameless: true,
		URL: "/", BackgroundColour: application.NewRGBA(30, 31, 34, 255), EnableFileDrop: true,
	})
	service.mainWindow = mainWindow
	mainWindow.OnWindowEvent(events.Common.WindowRuntimeReady, func(_ *application.WindowEvent) { service.applyWindowIcons() })
	mainWindow.OnWindowEvent(events.Common.WindowClosing, service.beforeMainClose)
	mainWindow.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		service.handleDroppedImages("main", event)
	})
	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}
