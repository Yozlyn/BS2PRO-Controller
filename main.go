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

// 获取WebView2用户数据目录路径
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
		Title:  "BS2PRO-控制台",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		// 拦截窗口关闭事件，隐藏到托盘
		OnBeforeClose: app.OnWindowClosing,
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
