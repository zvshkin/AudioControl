package main

import (
	"embed"

	"audio-control/config"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

// trayIconICO — та же иконка, что показывается в панели задач (Wails берёт её
// же для ресурса самого .exe). Встраивается в бинарник через go:embed, потому
// что собранное приложение не носит с собой исходный build/windows/icon.ico —
// он нужен только на этапе сборки.
//
//go:embed build/windows/icon.ico
var trayIconICO []byte

// startHiddenFromConfig читает настройку "запускать свёрнутым" ДО wails.Run.
// Опция StartHidden передаётся в options.App на старте и позже не меняется,
// поэтому полагаться на конфиг, который App загружает в startup(), уже поздно —
// окно к тому моменту показано. Читаем конфиг здесь отдельно и дёшево; ошибки
// намеренно игнорируем (нет конфига -> обычный видимый запуск).
func startHiddenFromConfig() bool {
	mgr, err := config.NewManager()
	if err != nil {
		return false
	}
	cfg, err := mgr.Load()
	if err != nil || cfg == nil {
		return false
	}
	return cfg.StartMinimized
}

func main() {

	initLogger()

	app := NewApp()
	startHidden := startHiddenFromConfig()

	err := wails.Run(&options.App{
		Title:             "AudioControl Central",
		Width:             920,
		Height:            640,
		MinWidth:          800,
		MinHeight:         580,
		DisableResize:     false,
		Frameless:         true,
		StartHidden:       startHidden,
		HideWindowOnClose: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 11, G: 13, B: 16, A: 255},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			BackdropType:         windows.Mica,
			Theme:                windows.Dark,
		},
	})

	if err != nil {

		appLog.Println("wails.Run failed:", err)
		showStartupError("AudioControl Central", "Не удалось запустить приложение:\n"+err.Error())
	}
}
