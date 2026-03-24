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
	"golang.org/x/sys/windows/registry"
)

//go:embed all:frontend/out
var assets embed.FS

//go:embed build/windows/icon.ico
var iconData []byte

// guiCapturePanic 捕获 GUI 进程异常，写入崩溃文件和日志
func guiCapturePanic(recovered any) {
	stack := debug.Stack()
	logDir := config.GetLogDir()
	_ = os.MkdirAll(logDir, 0755)

	fileName := fmt.Sprintf("crash_gui_%s.log", time.Now().Format("2006-01-02_15-04-05.000"))
	filePath := filepath.Join(logDir, fileName)

	var b strings.Builder
	b.WriteString("=== BS2PRO 图形界面崩溃报告 ===\n")
	b.WriteString(fmt.Sprintf("时间:      %s\n", time.Now().Format(time.RFC3339Nano)))
	b.WriteString(fmt.Sprintf("异常:      %v\n", recovered))
	b.WriteString(fmt.Sprintf("进程ID:    %d\n", os.Getpid()))
	b.WriteString(fmt.Sprintf("启动参数:  %v\n", os.Args))
	b.WriteString("\n--- 调用栈 ---\n")
	b.Write(stack)
	b.WriteString("\n")

	if err := os.WriteFile(filePath, []byte(b.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "写入GUI崩溃报告失败: %v\n异常: %v\n%s\n", err, recovered, stack)
	} else {
		fmt.Fprintf(os.Stderr, "GUI进程发生异常，崩溃报告已写入: %s\n", filePath)
	}

	// 同步写入运行日志
	if guiLogger != nil {
		guiLogger.Errorf("界面崩溃异常: %v", recovered)
		guiLogger.Errorf("界面崩溃调用栈:\n%s", string(stack))
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
		Title:           "BS2PRO-Controller",
		Width:           900,
		Height:          620,
		MinWidth:        900,
		MinHeight:       620,
		Frameless:       true,                // 无边框窗口
		CSSDragProperty: "--wails-draggable", // CSS拖拽属性
		CSSDragValue:    "drag",              // CSS拖拽值

		// 开机自启时直接藏入托盘，不弹出窗口
		StartHidden: isAutoStart,

		// 应用程序单实例锁
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "BS2PRO-Controller-Unique-Lock-2025",
			OnSecondInstanceLaunch: func(secondInstanceData options.SecondInstanceData) {
				if guiLogger != nil {
					guiLogger.Infof("检测到第二实例启动: args=%v", secondInstanceData.Args)
				}
				hasAutostart := false
				for _, arg := range secondInstanceData.Args {
					if arg == "--autostart" || arg == "-autostart" {
						hasAutostart = true
						break
					}
				}
				if !hasAutostart {
					if guiLogger != nil {
						guiLogger.Infof("第二实例触发主窗口唤起")
					}
					app.ShowWindow()
				} else if guiLogger != nil {
					guiLogger.Infof("第二实例为自启动参数，跳过唤起主窗口")
				}
			},
		},

		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: guiBackgroundColour(),
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
			Theme:                windows.SystemDefault, // 系统默认主题
		},
	})

	if err != nil {
		log.Fatal(err)
	}
}

func guiBackgroundColour() *options.RGBA {
	pref := loadThemePreference()
	if !pref.FollowSystem {
		if pref.Mode == "dark" {
			return &options.RGBA{R: 31, G: 31, B: 31, A: 255}
		}
		return &options.RGBA{R: 248, G: 250, B: 252, A: 255}
	}
	if prefersLightTheme() {
		return &options.RGBA{R: 248, G: 250, B: 252, A: 255}
	}
	return &options.RGBA{R: 31, G: 31, B: 31, A: 255}
}

func prefersLightTheme() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	value, _, err := key.GetIntegerValue("AppsUseLightTheme")
	if err != nil {
		return false
	}
	return value != 0
}
