package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
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

	// 自启动管理器，启动时初始化一次
	autostartManager *autostart.Manager
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
	logDir := config.GetLogDir()
	_ = os.MkdirAll(logDir, 0755)

	logFilePath := filepath.Join(logDir, fmt.Sprintf("gui_%s.log", time.Now().Format("2006-01-02")))

	encoderCfg := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		MessageKey:     "msg",
		CallerKey:      "caller",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	fileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    10,
		MaxBackups: 7,
		MaxAge:     7,
		Compress:   true,
	})

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		fileWriter,
		zapcore.InfoLevel,
	)

	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	guiLogger = logger.Sugar()
}

type trayLoggerAdapter struct {
	sugar      *zap.SugaredLogger
	installDir string
}

func (l *trayLoggerAdapter) Info(format string, v ...any)  { l.sugar.Infof(format, v...) }
func (l *trayLoggerAdapter) Error(format string, v ...any) { l.sugar.Errorf(format, v...) }
func (l *trayLoggerAdapter) Debug(format string, v ...any) { l.sugar.Debugf(format, v...) }
func (l *trayLoggerAdapter) Warn(format string, v ...any)  { l.sugar.Warnf(format, v...) }
func (l *trayLoggerAdapter) Close()                        { l.sugar.Sync() }
func (l *trayLoggerAdapter) CleanOldLogs()                 {}
func (l *trayLoggerAdapter) SetDebugMode(enabled bool)     {}

func (l *trayLoggerAdapter) GetLogDir() string {
	if l.installDir != "" {
		return filepath.Join(l.installDir, "logs")
	}
	return ""
}

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

	// 初始化自启动管理器
	adapter := &trayLoggerAdapter{sugar: guiLogger, installDir: config.GetInstallDir()}
	a.autostartManager = autostart.NewManager(adapter, config.GetInstallDir())

	// 连接到后台核心服务
	if err := a.ipcClient.Connect(); err != nil {
		guiLogger.Errorf("连接核心服务失败: %v", err)
		runtime.EventsEmit(ctx, "core-service-error", "无法连接到核心服务，请检查服务是否运行")

		go func() {
			defaultCfg := types.GetDefaultConfig(false)
			defaultCfg.WindowsAutoStart = a.autostartManager.CheckWindowsAutoStart()
			runtime.EventsEmit(ctx, "config-update", defaultCfg)
		}()
	} else {
		guiLogger.Info("已成功连接到核心服务 IPC 管道")
		a.ipcClient.SetEventHandler(a.handleCoreEvent)

		// 启动时主动拉取一次配置，同步状态
		cfg := a.GetConfig()
		status := a.GetDeviceStatus()
		cfg.WindowsAutoStart = a.autostartManager.CheckWindowsAutoStart()

		a.mutex.Lock()
		a.autoControlState = cfg.AutoControl
		if connected, ok := status["connected"].(bool); ok {
			a.isConnected = connected
		}
		a.mutex.Unlock()
		go func() {
			runtime.EventsEmit(ctx, "config-update", cfg)
		}()
	}

	// 初始化系统托盘
	a.InitSystemTray()

	// 启动连接健康检查
	go a.startConnectionHealthCheck()

	guiLogger.Info("=== BS2PRO GUI 启动完成 ===")
}

// InitSystemTray 初始化系统托盘
func (a *App) InitSystemTray() {
	trayAdapter := &trayLoggerAdapter{sugar: guiLogger, installDir: config.GetInstallDir()}
	a.trayManager = tray.NewManager(trayAdapter, a.iconData)

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
		func() {
			// 点击关闭核心：停止核心服务
			a.StopCoreService()
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
	defer func() { recover() }()
	if a.ctx == nil {
		return
	}

	guiLogger.Debug("handleCoreEvent: 收到事件类型=%v", event.Type)

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

	case ipc.EventServiceConnected:
		guiLogger.Info("核心服务连接事件 - UI 刷新")
		// 服务重新连接后，延迟半秒等待硬件和 IPC 管道彻底就绪
		go func() {
			time.Sleep(500 * time.Millisecond)
			cfg := a.GetConfig()
			status := a.GetDeviceStatus()

			a.mutex.Lock()
			if connected, ok := status["connected"].(bool); ok {
				a.isConnected = connected
			}
			a.autoControlState = cfg.AutoControl
			a.mutex.Unlock()

			if a.ctx != nil {
				// 发送恢复信号给前端
				runtime.EventsEmit(a.ctx, "core-service-connected", nil)
				runtime.EventsEmit(a.ctx, "config-update", cfg)

				// 如果核心服务汇报设备在线，一并通知前端设备在线
				if a.isConnected {
					runtime.EventsEmit(a.ctx, "device-connected", status["currentData"])
				}
			}
		}()

	case ipc.EventServiceDisconnected:
		guiLogger.Warn("核心服务断开事件")
		a.mutex.Lock()
		a.isConnected = false
		a.mutex.Unlock()

		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "core-service-error", "核心服务意外终止，正在尝试重连...")
			runtime.EventsEmit(a.ctx, "device-disconnected", nil)
		}

	case ipc.EventConfigUpdate:
		var cfg types.AppConfig
		if err := json.Unmarshal(event.Data, &cfg); err == nil {
			// 用注册表真实状态覆盖配置中的windowsAutoStart，保持两者一致
			cfg.WindowsAutoStart = a.CheckWindowsAutoStart()
			a.mutex.Lock()
			a.autoControlState = cfg.AutoControl
			a.mutex.Unlock()
			runtime.EventsEmit(a.ctx, "config-update", cfg)
		}

	case "show-window":
		a.ShowWindow()
	}
}

// sendRequest 发送请求到核心服务
func (a *App) sendRequest(reqType ipc.RequestType, data any) (*ipc.Response, error) {
	return a.ipcClient.SendRequest(reqType, data)
}

func (a *App) GetAppVersion() string { return version.Get() }

func (a *App) ConnectDevice() bool {
	resp, err := a.sendRequest(ipc.ReqConnect, nil)
	if err != nil || resp == nil || !resp.Success {
		return false
	}
	var success bool
	json.Unmarshal(resp.Data, &success)
	return success
}

func (a *App) DisconnectDevice() { a.sendRequest(ipc.ReqDisconnect, nil) }

func (a *App) GetDeviceStatus() map[string]any {
	resp, err := a.sendRequest(ipc.ReqGetDeviceStatus, nil)
	if err != nil || resp == nil || !resp.Success {
		return map[string]any{"connected": false}
	}
	var status map[string]any
	json.Unmarshal(resp.Data, &status)
	return status
}

func (a *App) GetConfig() AppConfig {
	resp, err := a.sendRequest(ipc.ReqGetConfig, nil)
	if err != nil || resp == nil || !resp.Success {
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
	if resp == nil || !resp.Success {
		if resp != nil {
			return fmt.Errorf("%s", resp.Error)
		}
		return fmt.Errorf("服务响应为空")
	}
	return nil
}

func (a *App) SetFanCurve(curve []FanCurvePoint) error {
	resp, err := a.sendRequest(ipc.ReqSetFanCurve, curve)
	if err != nil {
		return err
	}
	if resp == nil || !resp.Success {
		if resp != nil {
			return fmt.Errorf("%s", resp.Error)
		}
		return fmt.Errorf("服务响应为空")
	}
	return nil
}

func (a *App) GetFanCurve() []FanCurvePoint {
	resp, err := a.sendRequest(ipc.ReqGetFanCurve, nil)
	if err != nil || resp == nil || !resp.Success {
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
	if resp == nil || !resp.Success {
		if resp != nil {
			return fmt.Errorf("%s", resp.Error)
		}
		return fmt.Errorf("服务响应为空")
	}
	return nil
}

func (a *App) SetManualGear(gear, level string) bool {
	resp, err := a.sendRequest(ipc.ReqSetManualGear, ipc.SetManualGearParams{Gear: gear, Level: level})
	if err != nil || resp == nil || !resp.Success {
		return false
	}
	var success bool
	json.Unmarshal(resp.Data, &success)
	return success
}

func (a *App) GetAvailableGears() map[string][]GearCommand {
	resp, err := a.sendRequest(ipc.ReqGetAvailableGears, nil)
	if err != nil || resp == nil || !resp.Success {
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
	if resp == nil || !resp.Success {
		if resp != nil {
			return fmt.Errorf("%s", resp.Error)
		}
		return fmt.Errorf("服务响应为空")
	}
	return nil
}

func (a *App) SetGearLight(enabled bool) bool {
	resp, err := a.sendRequest(ipc.ReqSetGearLight, ipc.SetBoolParams{Enabled: enabled})
	if err != nil || resp == nil {
		return false
	}
	var success bool
	json.Unmarshal(resp.Data, &success)
	return success
}

func (a *App) SetPowerOnStart(enabled bool) bool {
	resp, err := a.sendRequest(ipc.ReqSetPowerOnStart, ipc.SetBoolParams{Enabled: enabled})
	if err != nil || resp == nil {
		return false
	}
	var success bool
	json.Unmarshal(resp.Data, &success)
	return success
}

func (a *App) SetSmartStartStop(mode string) bool {
	resp, err := a.sendRequest(ipc.ReqSetSmartStartStop, ipc.SetStringParams{Value: mode})
	if err != nil || resp == nil {
		return false
	}
	var success bool
	json.Unmarshal(resp.Data, &success)
	return success
}

func (a *App) SetBrightness(percentage int) bool {
	resp, err := a.sendRequest(ipc.ReqSetBrightness, ipc.SetIntParams{Value: percentage})
	if err != nil || resp == nil {
		return false
	}
	var success bool
	json.Unmarshal(resp.Data, &success)
	return success
}

func (a *App) SetRGBMode(params ipc.SetRGBModeParams) bool {
	resp, err := a.sendRequest(ipc.ReqSetRGBMode, params)
	if err != nil || resp == nil {
		return false
	}
	var success bool
	json.Unmarshal(resp.Data, &success)
	return success
}

func (a *App) GetTemperature() TemperatureData {
	resp, err := a.sendRequest(ipc.ReqGetTemperature, nil)
	if err != nil || resp == nil {
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
	if err != nil || resp == nil {
		return nil
	}
	var fanData FanData
	if err := json.Unmarshal(resp.Data, &fanData); err != nil {
		return nil
	}
	return &fanData
}

func (a *App) SetWindowsAutoStart(enable bool) error {
	if a.autostartManager == nil {
		// 防御性空指针保护
		adapter := &trayLoggerAdapter{sugar: guiLogger, installDir: config.GetInstallDir()}
		a.autostartManager = autostart.NewManager(adapter, config.GetInstallDir())
	}
	return a.autostartManager.SetWindowsAutoStart(enable)
}

func (a *App) CheckWindowsAutoStart() bool {
	if a.autostartManager == nil {
		adapter := &trayLoggerAdapter{sugar: guiLogger, installDir: config.GetInstallDir()}
		a.autostartManager = autostart.NewManager(adapter, config.GetInstallDir())
	}
	return a.autostartManager.CheckWindowsAutoStart()
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
		// Sync 将 zap 缓冲区写入磁盘，避免日志在os.Exit时丢失
		guiLogger.Sync()
		os.Exit(0)
	}()
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

// StopCoreService 停止核心服务
func (a *App) StopCoreService() bool {
	guiLogger.Info("控制台请求停止核心服务")
	resp, err := a.sendRequest(ipc.ReqStopService, nil)
	if err != nil {
		guiLogger.Errorf("发送停止核心服务请求失败: %v", err)
		return false
	} else if resp != nil && resp.Success {
		guiLogger.Info("核心服务停止请求已发送，服务将在后台异步停止")
		return true
	} else {
		guiLogger.Warn("停止核心服务请求未成功")
		return false
	}
}

func (a *App) TestTemperatureReading() TemperatureData {
	resp, err := a.sendRequest(ipc.ReqTestTemperatureReading, nil)
	if err != nil || resp == nil {
		return TemperatureData{}
	}
	var temp TemperatureData
	json.Unmarshal(resp.Data, &temp)
	return temp
}

func (a *App) TestBridgeProgram() BridgeTemperatureData {
	resp, err := a.sendRequest(ipc.ReqTestBridgeProgram, nil)
	if err != nil || resp == nil {
		errMsg := "请求失败"
		if err != nil {
			errMsg = err.Error()
		}
		return BridgeTemperatureData{Success: false, Error: errMsg}
	}
	var data BridgeTemperatureData
	json.Unmarshal(resp.Data, &data)
	return data
}

func (a *App) GetBridgeProgramStatus() map[string]any {
	resp, err := a.sendRequest(ipc.ReqGetBridgeProgramStatus, nil)
	if err != nil || resp == nil {
		errMsg := "请求失败"
		if err != nil {
			errMsg = err.Error()
		}
		return map[string]any{"error": errMsg}
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
	if err != nil || resp == nil {
		errMsg := "请求失败"
		if err != nil {
			errMsg = err.Error()
		}
		return map[string]any{"error": errMsg}
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
	if resp == nil || !resp.Success {
		if resp != nil {
			return fmt.Errorf("%s", resp.Error)
		}
		return fmt.Errorf("服务响应为空")
	}
	return nil
}

// LogFrontendError 接收前端上报的JS错误，写入gui日志文件
func (a *App) LogFrontendError(level, source, message, stack string) {
	if guiLogger == nil {
		return
	}
	entry := fmt.Sprintf("[frontend][%s] %s\n  stack: %s", source, message, stack)
	switch level {
	case "warn":
		guiLogger.Warn(entry)
	case "crash", "error":
		guiLogger.Error(entry)
	default:
		guiLogger.Info(entry)
	}
}

// startConnectionHealthCheck 启动连接健康检查
func (a *App) startConnectionHealthCheck() {
	guiLogger.Info("启动核心服务Watchdog")

	baseInterval := 3 * time.Second // 基础探测频率：3秒
	maxInterval := 30 * time.Second // 最大探测频率：30秒 (休眠期)
	currentInterval := baseInterval

	for {
		if !a.ipcClient.IsConnected() {
			guiLogger.Info("Watchdog: 检测到核心服务离线，尝试重连...")

			if err := a.ipcClient.Connect(); err == nil {
				guiLogger.Info("Watchdog: 核心服务重连成功！")
				currentInterval = baseInterval // 重连成功，重置为基础心跳频率
			} else {
				// 连接失败，推送UI状态
				if a.ctx != nil {
					runtime.EventsEmit(a.ctx, "core-service-error", "核心服务已停止，正在等待服务启动...")
					runtime.EventsEmit(a.ctx, "device-disconnected", nil)
				}

				// 指数退避，拉长下次探测的时间
				currentInterval *= 2
				if currentInterval > maxInterval {
					currentInterval = maxInterval
				}
				guiLogger.Debug("Watchdog: 重连失败，下次探测将在 %v 后进行", currentInterval)
			}
		} else {
			// 连接正常的情况下，发送Ping测活
			resp, err := a.sendRequest(ipc.ReqPing, nil)
			if err != nil || resp == nil || !resp.Success {
				guiLogger.Error("Watchdog: Ping 失败，判定管道假死，主动切断连接")
				a.ipcClient.Close()
				currentInterval = baseInterval // 准备立即开始快速重连
			} else {
				currentInterval = baseInterval // 保持正常的3秒心跳频率
			}
		}

		// 统一在此处休眠
		time.Sleep(currentInterval)
	}
}

// CheckConnectionStatus 检查当前连接状态（供前端调用）
func (a *App) CheckConnectionStatus() map[string]any {
	status := make(map[string]any)

	// 尝试发送Ping请求
	resp, err := a.sendRequest(ipc.ReqPing, nil)
	if err != nil {
		status["connected"] = false
		status["error"] = err.Error()
	} else {
		status["connected"] = resp != nil && resp.Success
		if resp != nil && !resp.Success {
			status["error"] = resp.Error
		}
	}

	return status
}
