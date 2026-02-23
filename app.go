package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/TIANLI0/BS2PRO-Controller/internal/autostart"
	"github.com/TIANLI0/BS2PRO-Controller/internal/config"
	"github.com/TIANLI0/BS2PRO-Controller/internal/ipc"
	"github.com/TIANLI0/BS2PRO-Controller/internal/tray"
	"github.com/TIANLI0/BS2PRO-Controller/internal/types"
	"github.com/TIANLI0/BS2PRO-Controller/internal/version"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"go.uber.org/zap"
)

// App struct - GUI 应用程序结构
type App struct {
	ctx         context.Context
	ipcClient   *ipc.Client
	mutex       sync.RWMutex
	trayManager *tray.Manager
	iconData    []byte

	// 缓存的状态 (托盘和前端随时读取)
	isConnected      bool
	currentTemp      types.TemperatureData
	currentFan       *types.FanData
	autoControlState bool
}

// 重新导出类型，供Wails生成TypeScript绑定
type (
	FanCurvePoint         = types.FanCurvePoint
	FanData               = types.FanData
	GearCommand           = types.GearCommand
	TemperatureData       = types.TemperatureData
	BridgeTemperatureData = types.BridgeTemperatureData
	AppConfig             = types.AppConfig
	RGBModeParams         = ipc.SetRGBModeParams
	RGBColorParam         = ipc.RGBColorParam
)

var guiLogger *zap.SugaredLogger

func init() {
	logger, _ := zap.NewProduction()
	guiLogger = logger.Sugar()
}

type trayLoggerAdapter struct {
	sugar *zap.SugaredLogger
}

func (l *trayLoggerAdapter) Info(format string, v ...any)  { l.sugar.Infof(format, v...) }
func (l *trayLoggerAdapter) Error(format string, v ...any) { l.sugar.Errorf(format, v...) }
func (l *trayLoggerAdapter) Debug(format string, v ...any) { l.sugar.Debugf(format, v...) }
func (l *trayLoggerAdapter) Warn(format string, v ...any)  { l.sugar.Warnf(format, v...) }
func (l *trayLoggerAdapter) Close()                        { l.sugar.Sync() }
func (l *trayLoggerAdapter) CleanOldLogs()                 {}
func (l *trayLoggerAdapter) SetDebugMode(enabled bool)     {}
func (l *trayLoggerAdapter) GetLogDir() string             { return "" }

// NewApp 创建 GUI 应用实例
func NewApp(icon []byte) *App {
	return &App{
		ipcClient:   ipc.NewClient(nil),
		currentTemp: types.TemperatureData{BridgeOk: true},
		iconData:    icon,
	}
}

// startup 应用启动时调用
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	guiLogger.Info("=== BS2PRO GUI 启动 ===")

	// 连接到后台核心服务
	if err := a.ipcClient.Connect(); err != nil {
		guiLogger.Errorf("连接核心服务失败: %v", err)
		runtime.EventsEmit(ctx, "core-service-error", "无法连接到核心服务，请检查服务是否运行")
	} else {
		guiLogger.Info("已成功连接到核心服务 IPC 管道")
		a.ipcClient.SetEventHandler(a.handleCoreEvent)

		// 启动时主动拉取一次配置，同步状态
		go func() {
			cfg := a.GetConfig()
			status := a.GetDeviceStatus()

			a.mutex.Lock()
			a.autoControlState = cfg.AutoControl
			if connected, ok := status["connected"].(bool); ok {
				a.isConnected = connected
			}
			a.mutex.Unlock()
		}()
	}

	// 初始化系统托盘
	a.InitSystemTray()

	guiLogger.Info("=== BS2PRO GUI 启动完成 ===")
}

// InitSystemTray 初始化系统托盘
func (a *App) InitSystemTray() {
	adapter := &trayLoggerAdapter{sugar: guiLogger}
	a.trayManager = tray.NewManager(adapter, a.iconData)

	a.trayManager.SetCallbacks(
		func() {
			// 左键双击托盘：显示窗口
			a.ShowWindow()
		},
		func() {
			// 点击退出：仅退出GUI进程
			a.QuitApp()
		},
		func() {
			// 点击重启服务：重启核心服务
			a.RestartCoreService()
		},
		func() bool {
			// 切换智能变频
			a.mutex.RLock()
			currentState := a.autoControlState
			a.mutex.RUnlock()

			newState := !currentState
			go a.SetAutoControl(newState)
			return newState
		},
		func() tray.Status {
			// 为托盘提供状态
			a.mutex.RLock()
			defer a.mutex.RUnlock()
			rpm := uint16(0)
			if a.currentFan != nil {
				rpm = uint16(a.currentFan.CurrentRPM)
			}
			return tray.Status{
				Connected:        a.isConnected,
				CPUTemp:          a.currentTemp.CPUTemp,
				GPUTemp:          a.currentTemp.GPUTemp,
				CurrentRPM:       rpm,
				AutoControlState: a.autoControlState,
			}
		},
	)

	a.trayManager.Init()
}

func (a *App) OnWindowClosing(ctx context.Context) bool {
	guiLogger.Info("拦截到窗口关闭动作，隐藏至托盘...")
	a.HideWindow()
	return true
}

// handleCoreEvent 处理核心服务推送的事件
func (a *App) handleCoreEvent(event ipc.Event) {
	if a.ctx == nil {
		return
	}

	switch event.Type {
	case ipc.EventFanDataUpdate:
		var fanData types.FanData
		if err := json.Unmarshal(event.Data, &fanData); err == nil {
			a.mutex.Lock()
			a.currentFan = &fanData
			a.mutex.Unlock()
			runtime.EventsEmit(a.ctx, "fan-data-update", fanData)
		}

	case ipc.EventTemperatureUpdate:
		var temp types.TemperatureData
		if err := json.Unmarshal(event.Data, &temp); err == nil {
			a.mutex.Lock()
			a.currentTemp = temp
			a.mutex.Unlock()
			runtime.EventsEmit(a.ctx, "temperature-update", temp)
		}

	case ipc.EventDeviceConnected:
		var deviceInfo map[string]string
		json.Unmarshal(event.Data, &deviceInfo)
		a.mutex.Lock()
		a.isConnected = true
		a.mutex.Unlock()
		runtime.EventsEmit(a.ctx, "device-connected", deviceInfo)

	case ipc.EventDeviceDisconnected:
		a.mutex.Lock()
		a.isConnected = false
		a.mutex.Unlock()
		runtime.EventsEmit(a.ctx, "device-disconnected", nil)

	case ipc.EventDeviceError:
		var errMsg string
		json.Unmarshal(event.Data, &errMsg)
		runtime.EventsEmit(a.ctx, "device-error", errMsg)

	case ipc.EventConfigUpdate:
		var cfg types.AppConfig
		if err := json.Unmarshal(event.Data, &cfg); err == nil {
			a.mutex.Lock()
			a.autoControlState = cfg.AutoControl
			a.mutex.Unlock()
			runtime.EventsEmit(a.ctx, "config-update", cfg)
		}
	}
}

// sendRequest 发送请求到核心服务
func (a *App) sendRequest(reqType ipc.RequestType, data any) (*ipc.Response, error) {
	return a.ipcClient.SendRequest(reqType, data)
}

func (a *App) GetAppVersion() string { return version.Get() }

func (a *App) ConnectDevice() bool {
	resp, err := a.sendRequest(ipc.ReqConnect, nil)
	if err != nil || !resp.Success {
		return false
	}
	var success bool
	json.Unmarshal(resp.Data, &success)
	return success
}

func (a *App) DisconnectDevice() { a.sendRequest(ipc.ReqDisconnect, nil) }

func (a *App) GetDeviceStatus() map[string]any {
	resp, err := a.sendRequest(ipc.ReqGetDeviceStatus, nil)
	if err != nil || !resp.Success {
		return map[string]any{"connected": false}
	}
	var status map[string]any
	json.Unmarshal(resp.Data, &status)
	return status
}

func (a *App) GetConfig() AppConfig {
	resp, err := a.sendRequest(ipc.ReqGetConfig, nil)
	if err != nil || !resp.Success {
		return types.GetDefaultConfig(false)
	}
	var cfg AppConfig
	json.Unmarshal(resp.Data, &cfg)
	return cfg
}

func (a *App) UpdateConfig(cfg AppConfig) error {
	resp, err := a.sendRequest(ipc.ReqUpdateConfig, cfg)
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

func (a *App) SetFanCurve(curve []FanCurvePoint) error {
	resp, err := a.sendRequest(ipc.ReqSetFanCurve, curve)
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

func (a *App) GetFanCurve() []FanCurvePoint {
	resp, err := a.sendRequest(ipc.ReqGetFanCurve, nil)
	if err != nil || !resp.Success {
		return types.GetDefaultFanCurve()
	}
	var curve []FanCurvePoint
	json.Unmarshal(resp.Data, &curve)
	return curve
}

func (a *App) SetAutoControl(enabled bool) error {
	resp, err := a.sendRequest(ipc.ReqSetAutoControl, ipc.SetAutoControlParams{Enabled: enabled})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

func (a *App) SetManualGear(gear, level string) bool {
	resp, err := a.sendRequest(ipc.ReqSetManualGear, ipc.SetManualGearParams{Gear: gear, Level: level})
	if err != nil || !resp.Success {
		return false
	}
	var success bool
	json.Unmarshal(resp.Data, &success)
	return success
}

func (a *App) GetAvailableGears() map[string][]GearCommand {
	resp, err := a.sendRequest(ipc.ReqGetAvailableGears, nil)
	if err != nil || !resp.Success {
		return types.GearCommands
	}
	var gears map[string][]GearCommand
	json.Unmarshal(resp.Data, &gears)
	return gears
}

func (a *App) SetCustomSpeed(enabled bool, rpm int) error {
	resp, err := a.sendRequest(ipc.ReqSetCustomSpeed, ipc.SetCustomSpeedParams{Enabled: enabled, RPM: rpm})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

func (a *App) SetGearLight(enabled bool) bool {
	resp, err := a.sendRequest(ipc.ReqSetGearLight, ipc.SetBoolParams{Enabled: enabled})
	if err != nil {
		return false
	}
	var success bool
	json.Unmarshal(resp.Data, &success)
	return success
}

func (a *App) SetPowerOnStart(enabled bool) bool {
	resp, err := a.sendRequest(ipc.ReqSetPowerOnStart, ipc.SetBoolParams{Enabled: enabled})
	if err != nil {
		return false
	}
	var success bool
	json.Unmarshal(resp.Data, &success)
	return success
}

func (a *App) SetSmartStartStop(mode string) bool {
	resp, err := a.sendRequest(ipc.ReqSetSmartStartStop, ipc.SetStringParams{Value: mode})
	if err != nil {
		return false
	}
	var success bool
	json.Unmarshal(resp.Data, &success)
	return success
}

func (a *App) SetBrightness(percentage int) bool {
	resp, err := a.sendRequest(ipc.ReqSetBrightness, ipc.SetIntParams{Value: percentage})
	if err != nil {
		return false
	}
	var success bool
	json.Unmarshal(resp.Data, &success)
	return success
}

func (a *App) SetRGBMode(params ipc.SetRGBModeParams) bool {
	resp, err := a.sendRequest(ipc.ReqSetRGBMode, params)
	if err != nil {
		return false
	}
	var success bool
	json.Unmarshal(resp.Data, &success)
	return success
}

func (a *App) GetTemperature() TemperatureData {
	resp, err := a.sendRequest(ipc.ReqGetTemperature, nil)
	if err != nil {
		a.mutex.RLock()
		defer a.mutex.RUnlock()
		return a.currentTemp
	}
	var temp TemperatureData
	json.Unmarshal(resp.Data, &temp)
	return temp
}

func (a *App) GetCurrentFanData() *FanData {
	resp, err := a.sendRequest(ipc.ReqGetCurrentFanData, nil)
	if err != nil {
		return nil
	}
	var fanData FanData
	if err := json.Unmarshal(resp.Data, &fanData); err != nil {
		return nil
	}
	return &fanData
}

func (a *App) SetWindowsAutoStart(enable bool) error {
	adapter := &trayLoggerAdapter{sugar: guiLogger}
	installDir := config.GetInstallDir()
	manager := autostart.NewManager(adapter, installDir)
	return manager.SetWindowsAutoStart(enable)
}

func (a *App) CheckWindowsAutoStart() bool {
	adapter := &trayLoggerAdapter{sugar: guiLogger}
	installDir := config.GetInstallDir()
	manager := autostart.NewManager(adapter, installDir)
	return manager.CheckWindowsAutoStart()
}

func (a *App) IsAutoStartLaunch() bool {
	return autostart.DetectAutoStartLaunch(os.Args)
}

func (a *App) ShowWindow() {
	if a.ctx != nil {
		runtime.WindowShow(a.ctx)
	}
}

func (a *App) HideWindow() {
	if a.ctx != nil {
		runtime.WindowHide(a.ctx)
	}
}

func (a *App) QuitApp() {
	guiLogger.Info("控制台请求退出")
	if a.trayManager != nil {
		a.trayManager.Quit()
	}
	if a.ipcClient != nil {
		a.ipcClient.Close()
	}
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}

	go func() {
		time.Sleep(500 * time.Millisecond)
		guiLogger.Info("执行强杀...")
		os.Exit(0)
	}()
}

func (a *App) QuitAll() {
	guiLogger.Info("GUI 彻底退出")
	a.sendRequest(ipc.ReqQuitApp, nil)
	a.QuitApp()
}

// QuitServiceOnly 只退出核心服务，不关闭GUI界面
func (a *App) QuitServiceOnly() {
	guiLogger.Info("GUI 请求只退出核心服务")
	resp, err := a.sendRequest(ipc.ReqQuitApp, nil)
	if err != nil {
		guiLogger.Errorf("发送退出核心服务请求失败: %v", err)
	} else if resp != nil && resp.Success {
		guiLogger.Info("核心服务已退出")
	} else {
		guiLogger.Warn("退出核心服务请求未成功")
	}
}

// RestartCoreService 重启核心服务
func (a *App) RestartCoreService() bool {
	guiLogger.Info("控制台请求重启核心服务")
	resp, err := a.sendRequest(ipc.ReqRestartService, nil)
	if err != nil {
		guiLogger.Errorf("发送重启核心服务请求失败: %v", err)
		return false
	} else if resp != nil && resp.Success {
		guiLogger.Info("核心服务重启请求已发送，服务将在后台异步重启")
		return true
	} else {
		guiLogger.Warn("重启核心服务请求未成功")
		return false
	}
}

func (a *App) TestTemperatureReading() TemperatureData {
	resp, err := a.sendRequest(ipc.ReqTestTemperatureReading, nil)
	if err != nil {
		return TemperatureData{}
	}
	var temp TemperatureData
	json.Unmarshal(resp.Data, &temp)
	return temp
}

func (a *App) TestBridgeProgram() BridgeTemperatureData {
	resp, err := a.sendRequest(ipc.ReqTestBridgeProgram, nil)
	if err != nil {
		return BridgeTemperatureData{Success: false, Error: err.Error()}
	}
	var data BridgeTemperatureData
	json.Unmarshal(resp.Data, &data)
	return data
}

func (a *App) GetBridgeProgramStatus() map[string]any {
	resp, err := a.sendRequest(ipc.ReqGetBridgeProgramStatus, nil)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	var status map[string]any
	json.Unmarshal(resp.Data, &status)
	return status
}

func (a *App) UpdateGuiResponseTime() {
	a.sendRequest(ipc.ReqUpdateGuiResponseTime, nil)
}

func (a *App) GetDebugInfo() map[string]any {
	resp, err := a.sendRequest(ipc.ReqGetDebugInfo, nil)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	var info map[string]any
	json.Unmarshal(resp.Data, &info)
	return info
}

func (a *App) SetDebugMode(enabled bool) error {
	resp, err := a.sendRequest(ipc.ReqSetDebugMode, ipc.SetBoolParams{Enabled: enabled})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}
