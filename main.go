package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/out
var assets embed.FS

//go:embed build/windows/icon.ico
var iconData []byte

// 获取WebView2用户数据目录路径，隔离缓存以便卸载时干净清理
func getWebView2DataPath() string {
	appData, err := os.UserConfigDir()
	if err != nil {
		homeDir, _ := os.UserHomeDir()
		appData = filepath.Join(homeDir, "AppData", "Roaming")
	}
	return filepath.Join(appData, "BS2PRO-Controller")
}

func main() {
	app := NewApp(iconData)

	// 启动 Wails 框架
	err := wails.Run(&options.App{
		Title:     "BS2PRO-控制台",
		Width:     1024,
		Height:    680,
		MinWidth:  850,
		MinHeight: 600,

		StartHidden: false,

		// 应用程序单实例锁
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "BS2PRO-Controller-Unique-Lock-2025",
			OnSecondInstanceLaunch: func(secondInstanceData options.SecondInstanceData) {
				hasAutostart := false
				for _, arg := range secondInstanceData.Args {
					if arg == "--autostart" || arg == "-autostart" {
						hasAutostart = true
						break
					}
				}
				if !hasAutostart {
					app.ShowWindow()
				}
			},
		},

		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnBeforeClose:    app.OnWindowClosing,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
			WebviewUserDataPath:  getWebView2DataPath(),
		},
	})

	if err != nil {
		log.Fatal(err)
	}
}
