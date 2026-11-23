package main

// Wisteria Desktop — Windows/Linux GUI client for the wisteria protocol, with
// sing-box/xray based support for the common proxy protocols (ss/vmess/vless/
// trojan/hysteria2/tuic). Architecture: a GUI shell supervising core
// binaries as subprocesses (the v2rayN model); the wisteria core is the
// same wisteria-client the repo ships.

import (
	"embed"
	"fmt"
	"os"
	"runtime"

	"github.com/qqkane/wisteria-app/desktop/internal/core"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed frontend/dist/assets/icon.png
var appIconPNG []byte

func main() {
	core.SetAppIcon(appIconPNG)
	if runtime.GOOS == "linux" {
		// WebKit2GTK's DMABUF renderer is pathologically slow on NVIDIA
		// proprietary setups (CSS animations drop to ~20-30 fps, sometimes
		// with flicker). Must be set before WebKit loads; the user can still
		// override with their own value.
		if os.Getenv("WEBKIT_DISABLE_DMABUF_RENDERER") == "" {
			os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")
		}
	}
	app := NewApp()
	err := wails.Run(&options.App{
		Title:            "Wisteria Desktop",
		Width:            1020,
		Height:           700,
		MinWidth:         820,
		MinHeight:        560,
		BackgroundColour: &options.RGBA{R: 20, G: 16, B: 31, A: 255},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:     app.startup,
		OnBeforeClose: app.beforeClose,
		OnShutdown:    app.shutdown,
		Bind:          []interface{}{app},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			Theme:                windows.Dark,
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}
