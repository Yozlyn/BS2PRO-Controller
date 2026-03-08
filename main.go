package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/TIANLI0/BS2PRO-Controller/internal/config"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/out
var assets embed.FS

//go:embed build/windows/icon.ico
var iconData []byte

// guiCapturePanic 捕获GUI进程的panic，写入crash文件和日志
func guiCapturePanic(recovered any) {
	stack := debug.Stack()
	logDir := config.GetLogDir()
	_ = os.MkdirAll(logDir, 0755)

	fileName := fmt.Sprintf("crash_gui_%s.log", time.Now().Format("2006-01-02_15-04-05.000"))
	filePath := filepath.Join(logDir, fileName)

	var b strings.Builder
	b.WriteString("=== BS2PRO GUI Crash Report ===\n")
	b.WriteString(fmt.Sprintf("time:  %s\n", time.Now().Format(time.RFC3339Nano)))
	b.WriteString(fmt.Sprintf("panic: %v\n", recovered))
	b.WriteString(fmt.Sprintf("pid:   %d\n", os.Getpid()))
	b.WriteString(fmt.Sprintf("args:  %v\n", os.Args))
	b.WriteString("\n--- stack ---\n")
	b.Write(stack)
	b.WriteString("\n")

	if err := os.WriteFile(filePath, []byte(b.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "写入GUI崩溃报告失败: %v\npanic: %v\n%s\n", err, recovered, stack)
	} else {
		fmt.Fprintf(os.Stderr, "GUI进程发生panic，崩溃报告已写入: %s\n", filePath)
	}

	// 同步写入运行日志
	if guiLogger != nil {
		guiLogger.Errorf("[GUI crash] panic: %v", recovered)
		guiLogger.Errorf("[GUI crash] stack:\n%s", string(stack))
		guiLogger.Sync()
	}
}

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
	defer func() {
		if r := recover(); r != nil {
			guiCapturePanic(r)
			os.Exit(1)
		}
	}()

	app := NewApp(iconData)

	// 检测是否为开机自启动模式
	isAutoStart := false
	for _, arg := range os.Args[1:] {
		if arg == "--autostart" || arg == "/autostart" || arg == "-autostart" {
			isAutoStart = true
			break
		}
	}

	// 启动 Wails 框架
	err := wails.Run(&options.App{
		Title:     "BS2PRO-控制台",
		Width:     1024,
		Height:    720,
		MinWidth:  850,
		MinHeight: 600,

		// 开机自启时直接藏入托盘，不弹出窗口
		StartHidden: isAutoStart,

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
