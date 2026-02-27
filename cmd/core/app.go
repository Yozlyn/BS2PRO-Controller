package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/TIANLI0/BS2PRO-Controller/internal/asus"
	"github.com/TIANLI0/BS2PRO-Controller/internal/config"
	"github.com/TIANLI0/BS2PRO-Controller/internal/device"
	"github.com/TIANLI0/BS2PRO-Controller/internal/ipc"
	"github.com/TIANLI0/BS2PRO-Controller/internal/logger"
	"github.com/TIANLI0/BS2PRO-Controller/internal/rgb"
	"github.com/TIANLI0/BS2PRO-Controller/internal/temperature"
	"github.com/TIANLI0/BS2PRO-Controller/internal/types"
	"github.com/TIANLI0/BS2PRO-Controller/internal/version"
)

type CoreApp struct {
	ctx context.Context

	deviceManager *device.Manager
	asusClient    *asus.Client
	tempReader    *temperature.Reader
	configManager *config.Manager
	logger        *logger.CustomLogger
	ipcServer     *ipc.Server

	isConnected        bool
	monitoringTemp     bool
	userDisconnected   bool
	currentTemp        types.TemperatureData
	lastDeviceMode     string
	userSetAutoControl bool
	debugMode          bool

	guiLastResponse   int64
	guiMonitorEnabled bool
	healthCheckTicker *time.Ticker
	cleanupChan       chan bool

	mutex          sync.RWMutex
	stopMonitoring chan bool

	// 记录当前已经下发的 RGB 智能温度档位
	lastSmartModeLevel byte
}

func NewCoreApp(debugMode bool) *CoreApp {
	installDir := config.GetInstallDir()
	// 日志统一写入 ProgramData\BS2PRO-Controller\logs，与 GUI 进程保持一致
	logBaseDir := filepath.Dir(config.GetLogDir()) // ProgramData\BS2PRO-Controller
	customLogger, err := logger.NewCustomLogger(debugMode, logBaseDir)
	if err != nil {
		// 降级：尝试系统临时目录，避免panic导致崩溃报告无法写入
		fallbackDir := os.TempDir()
		customLogger, err = logger.NewCustomLogger(debugMode, fallbackDir)
		if err != nil {
			// 最坏情况：创建一个只写stderr的logger，保证后续代码不会nil panic
			customLogger, _ = logger.NewCustomLogger(debugMode, ".")
		}
		if customLogger != nil {
			customLogger.Warn("日志目录初始化失败，已降级到临时目录: %s", fallbackDir)
		}
	} else {
		customLogger.Info("核心服务启动")
		customLogger.Info("安装目录: %s", installDir)
		customLogger.CleanOldLogs()
	}

	asusClient, err := asus.NewClient()
	if err != nil {
		customLogger.Warn("ASUS ACPI 客户端初始化失败: %v", err)
	}

	deviceMgr := device.NewManager(customLogger)
	tempReader := temperature.NewReader(asusClient, customLogger)
	configMgr := config.NewManager(installDir, customLogger)

	app := &CoreApp{
		ctx:                context.Background(),
		deviceManager:      deviceMgr,
		asusClient:         asusClient,
		tempReader:         tempReader,
		currentTemp:        types.TemperatureData{BridgeOk: true},
		configManager:      configMgr,
		logger:             customLogger,
		isConnected:        false,
		monitoringTemp:     false,
		stopMonitoring:     make(chan bool, 1),
		lastDeviceMode:     "",
		userSetAutoControl: false,
		debugMode:          debugMode,
		guiLastResponse:    time.Now().Unix(),
		cleanupChan:        make(chan bool, 1),
		guiMonitorEnabled:  true,
		lastSmartModeLevel: 0,
	}
	return app
}

func (a *CoreApp) Start() error {
	a.logInfo("=== BS2PRO 核心服务(Windows Service) 启动 ===")
	a.logInfo("版本: %s", version.Get())

	cfg := a.configManager.Load(false)
	if cfg.DebugMode {
		a.debugMode = true
		if a.logger != nil {
			a.logger.SetDebugMode(true)
		}
	}

	if err := a.deviceManager.Init(); err != nil {
		a.logError("初始化HID库失败: %v", err)
		return err
	}
	a.deviceManager.SetCallbacks(a.onFanDataUpdate, a.onDeviceDisconnect)

	a.logInfo("启动 IPC 服务器 (Named Pipe)")
	a.ipcServer = ipc.NewServer(a.handleIPCRequest, a.logger)
	if err := a.ipcServer.Start(); err != nil {
		a.logError("启动 IPC 服务器失败: %v", err)
		return err
	}

	if cfg.GuiMonitoring {
		a.logInfo("启动健康监控")
		a.safeGo("startHealthMonitoring", func() {
			a.startHealthMonitoring()
		})
	}

	a.safeGo("delayedConnectDevice", func() {
		time.Sleep(1 * time.Second)
		a.ConnectDevice()
	})

	return nil
}

func (a *CoreApp) Stop() {
	a.logInfo("核心服务正在停止...")
	a.cleanup()
	a.DisconnectDevice()
	if a.asusClient != nil {
		a.asusClient.Close()
	}
	if a.ipcServer != nil {
		a.ipcServer.Stop()
	}
	a.logInfo("核心服务已停止")
}

func (a *CoreApp) onShowWindowRequest() {
	a.logInfo("收到显示窗口请求")
	if a.ipcServer != nil && a.ipcServer.HasClients() {
		a.ipcServer.BroadcastEvent("show-window", nil)
	} else {
		a.logInfo("没有 GUI 连接，服务模式下无法主动唤起窗口。")
	}
}

func (a *CoreApp) onQuitRequest() {
	a.logInfo("收到前端的彻底退出请求，准备关闭核心服务...")
	if a.ipcServer != nil {
		a.ipcServer.BroadcastEvent("quit", nil)
	}

	go func() {
		defer func() { recover() }()
		time.Sleep(1 * time.Second)
		a.Stop() // 释放硬件句柄
		a.logInfo("核心服务进程自我终止")
		os.Exit(0) // 正常退出
	}()
}

func (a *CoreApp) handleIPCRequest(req ipc.Request) (res ipc.Response) {
	defer func() {
		if r := recover(); r != nil {
			a.logError("处理 IPC 请求时发生致命异常: %v", r)
			res = a.errorResponse(fmt.Sprintf("内部异常: %v", r))
		}
	}()

	switch req.Type {
	case ipc.ReqConnect:
		success := a.ConnectDevice()
		return a.successResponse(success)
	case ipc.ReqDisconnect:
		a.DisconnectDevice()
		return a.successResponse(true)
	case ipc.ReqGetDeviceStatus:
		status := a.GetDeviceStatus()
		return a.dataResponse(status)
	case ipc.ReqGetCurrentFanData:
		data := a.deviceManager.GetCurrentFanData()
		return a.dataResponse(data)
	case ipc.ReqGetConfig:
		cfg := a.configManager.Get()
		return a.dataResponse(cfg)
	case ipc.ReqUpdateConfig:
		var cfg types.AppConfig
		if err := json.Unmarshal(req.Data, &cfg); err != nil {
			return a.errorResponse("解析配置失败: " + err.Error())
		}
		if err := a.UpdateConfig(cfg); err != nil {
			return a.errorResponse(err.Error())
		}
		return a.successResponse(true)
	case ipc.ReqSetFanCurve:
		var curve []types.FanCurvePoint
		if err := json.Unmarshal(req.Data, &curve); err != nil {
			return a.errorResponse("解析风扇曲线失败: " + err.Error())
		}
		if err := a.SetFanCurve(curve); err != nil {
			return a.errorResponse(err.Error())
		}
		return a.successResponse(true)
	case ipc.ReqGetFanCurve:
		curve := a.configManager.Get().FanCurve
		return a.dataResponse(curve)
	case ipc.ReqSetAutoControl:
		var params ipc.SetAutoControlParams
		if err := json.Unmarshal(req.Data, &params); err != nil {
			return a.errorResponse("解析参数失败: " + err.Error())
		}
		if err := a.SetAutoControl(params.Enabled); err != nil {
			return a.errorResponse(err.Error())
		}
		return a.successResponse(true)
	case ipc.ReqSetManualGear:
		var params ipc.SetManualGearParams
		if err := json.Unmarshal(req.Data, &params); err != nil {
			return a.errorResponse("解析参数失败: " + err.Error())
		}
		success := a.SetManualGear(params.Gear, params.Level)
		return a.successResponse(success)
	case ipc.ReqGetAvailableGears:
		gears := types.GearCommands
		return a.dataResponse(gears)
	case ipc.ReqSetCustomSpeed:
		var params ipc.SetCustomSpeedParams
		if err := json.Unmarshal(req.Data, &params); err != nil {
			return a.errorResponse("解析参数失败: " + err.Error())
		}
		if err := a.SetCustomSpeed(params.Enabled, params.RPM); err != nil {
			return a.errorResponse(err.Error())
		}
		return a.successResponse(true)
	case ipc.ReqSetGearLight:
		var params ipc.SetBoolParams
		if err := json.Unmarshal(req.Data, &params); err != nil {
			return a.errorResponse("解析参数失败: " + err.Error())
		}
		success := a.SetGearLight(params.Enabled)
		return a.successResponse(success)
	case ipc.ReqSetPowerOnStart:
		var params ipc.SetBoolParams
		if err := json.Unmarshal(req.Data, &params); err != nil {
			return a.errorResponse("解析参数失败: " + err.Error())
		}
		success := a.SetPowerOnStart(params.Enabled)
		return a.successResponse(success)
	case ipc.ReqSetSmartStartStop:
		var params ipc.SetStringParams
		if err := json.Unmarshal(req.Data, &params); err != nil {
			return a.errorResponse("解析参数失败: " + err.Error())
		}
		success := a.SetSmartStartStop(params.Value)
		return a.successResponse(success)
	case ipc.ReqSetBrightness:
		var params ipc.SetIntParams
		if err := json.Unmarshal(req.Data, &params); err != nil {
			return a.errorResponse("解析参数失败: " + err.Error())
		}
		success := a.SetBrightness(params.Value)
		return a.successResponse(success)
	case ipc.ReqGetTemperature:
		a.mutex.RLock()
		temp := a.currentTemp
		a.mutex.RUnlock()
		return a.dataResponse(temp)
	case ipc.ReqTestTemperatureReading:
		temp := a.tempReader.Read()
		return a.dataResponse(temp)
	case ipc.ReqTestBridgeProgram:
		var data types.BridgeTemperatureData
		if a.asusClient != nil {
			cpuTemp, err := a.asusClient.GetCPUTemperature()
			if err == nil && cpuTemp > 0 && cpuTemp < 150 {
				data = types.BridgeTemperatureData{
					CpuTemp:    cpuTemp,
					GpuTemp:    0,
					MaxTemp:    cpuTemp,
					UpdateTime: time.Now().Unix(),
					Success:    true,
					Error:      "",
				}
			} else {
				data = types.BridgeTemperatureData{Success: false, Error: fmt.Sprintf("ASUS ACPI测试失败: %v", err)}
			}
		} else {
			data = types.BridgeTemperatureData{Success: false, Error: "ASUS ACPI客户端未初始化"}
		}
		return a.dataResponse(data)
	case ipc.ReqGetBridgeProgramStatus:
		var status map[string]interface{}
		if a.asusClient != nil {
			status = map[string]interface{}{"running": true, "status": "ASUS ACPI接口运行中", "type": "asus_acpi"}
		} else {
			status = map[string]interface{}{"running": false, "status": "ASUS ACPI接口未初始化", "type": "none"}
		}
		return a.dataResponse(status)
	case ipc.ReqShowWindow:
		a.onShowWindowRequest()
		return a.successResponse(true)
	case ipc.ReqHideWindow:
		return a.successResponse(true)
	case ipc.ReqQuitApp:
		go a.onQuitRequest()
		return a.successResponse(true)
	case ipc.ReqGetDebugInfo:
		info := a.GetDebugInfo()
		return a.dataResponse(info)
	case ipc.ReqSetDebugMode:
		var params ipc.SetBoolParams
		if err := json.Unmarshal(req.Data, &params); err != nil {
			return a.errorResponse("解析参数失败: " + err.Error())
		}
		if err := a.SetDebugMode(params.Enabled); err != nil {
			return a.errorResponse(err.Error())
		}
		return a.successResponse(true)
	case ipc.ReqUpdateGuiResponseTime:
		atomic.StoreInt64(&a.guiLastResponse, time.Now().Unix())
		return a.successResponse(true)
	case ipc.ReqPing:
		return a.dataResponse("pong")
	case ipc.ReqSetRGBMode:
		var params ipc.SetRGBModeParams
		if err := json.Unmarshal(req.Data, &params); err != nil {
			return a.errorResponse("解析RGB参数失败: " + err.Error())
		}
		success := a.SetRGBMode(params)
		return a.successResponse(success)
	case ipc.ReqRestartService:
		success := a.RestartService()
		return a.successResponse(success)
	case ipc.ReqStopService:
		success := a.StopService()
		return a.successResponse(success)
	case ipc.ReqUnsubscribeEvents:
		return a.successResponse(true)
	default:
		return a.errorResponse(fmt.Sprintf("未知的请求类型: %s", req.Type))
	}
}

func (a *CoreApp) successResponse(success bool) ipc.Response {
	data, _ := json.Marshal(success)
	return ipc.Response{Success: true, Data: data}
}

func (a *CoreApp) errorResponse(errMsg string) ipc.Response {
	return ipc.Response{Success: false, Error: errMsg}
}

func (a *CoreApp) dataResponse(data any) ipc.Response {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return a.errorResponse("序列化数据失败: " + err.Error())
	}
	return ipc.Response{Success: true, Data: dataBytes}
}

func (a *CoreApp) onFanDataUpdate(fanData *types.FanData) {
	a.mutex.Lock()
	cfg := a.configManager.Get()
	var shouldBroadcastConfig bool
	var broadcastCfg types.AppConfig
	if fanData.WorkMode == "挡位工作模式" && cfg.AutoControl && a.lastDeviceMode == "自动模式(实时转速)" && !a.userSetAutoControl && !cfg.IgnoreDeviceOnReconnect {
		a.logInfo("检测到设备从自动模式切换到挡位工作模式，自动关闭智能变频")
		cfg.AutoControl = false
		if a.monitoringTemp {
			select {
			case a.stopMonitoring <- true:
			default:
			}
		}
		a.configManager.Set(cfg)
		a.configManager.Save()
		shouldBroadcastConfig = true
		broadcastCfg = cfg
	} else if fanData.WorkMode == "挡位工作模式" && cfg.AutoControl && a.lastDeviceMode == "自动模式(实时转速)" && !a.userSetAutoControl && cfg.IgnoreDeviceOnReconnect {
		a.logInfo("检测到设备模式变化，但已开启断连保持配置模式，保持APP配置不变")
	}

	a.lastDeviceMode = fanData.WorkMode
	if a.userSetAutoControl {
		a.userSetAutoControl = false
	}
	a.mutex.Unlock()

	// 在锁外进行广播，避免持锁期间阻塞
	if shouldBroadcastConfig && a.ipcServer != nil {
		a.ipcServer.BroadcastEvent(ipc.EventConfigUpdate, broadcastCfg)
	}
	if a.ipcServer != nil {
		a.ipcServer.BroadcastEvent(ipc.EventFanDataUpdate, fanData)
	}
}

func (a *CoreApp) onDeviceDisconnect() {
	a.mutex.Lock()
	wasConnected := a.isConnected
	a.isConnected = false
	userDid := a.userDisconnected
	a.mutex.Unlock()

	if wasConnected {
		if userDid {
			a.logInfo("设备连接已主动断开")
		} else {
			a.logInfo("设备连接意外断开，将尝试自动重连")
		}
	}

	if a.ipcServer != nil {
		a.ipcServer.BroadcastEvent(ipc.EventDeviceDisconnected, nil)
	}

	if !userDid {
		go a.scheduleReconnect()
	}
}

func (a *CoreApp) scheduleReconnect() {
	defer func() {
		if r := recover(); r != nil {
			a.logError("自动重连时发生Panic: %v", r)
		}
	}()

	retryDelays := []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second}
	for i, delay := range retryDelays {
		a.mutex.RLock()
		connected := a.isConnected
		a.mutex.RUnlock()
		if connected {
			return
		}

		a.logInfo("等待 %v 后尝试第 %d 次重连...", delay, i+1)
		time.Sleep(delay)

		a.mutex.RLock()
		connected = a.isConnected
		a.mutex.RUnlock()
		if connected {
			return
		}

		if a.ConnectDevice() {
			a.logInfo("设备重连成功")
			cfg := a.configManager.Get()
			if cfg.IgnoreDeviceOnReconnect {
				a.logInfo("断连保持配置模式已开启，重新应用APP配置")
				a.reapplyConfigAfterReconnect()
			}
			return
		}
		a.logError("第 %d 次重连失败", i+1)
	}
}

func (a *CoreApp) reapplyConfigAfterReconnect() {
	a.logInfo("设备重连后重新应用配置")
	a.applyConfigOnConnect()
	a.logInfo("重连后配置重新应用完成")
}

func (a *CoreApp) applyConfigOnConnect() {
	cfg := a.configManager.Get()
	a.logInfo("开始应用配置到设备")

	time.Sleep(200 * time.Millisecond)

	if !cfg.AutoControl {
		if cfg.ManualGear != "" && cfg.ManualLevel != "" {
			for i := 0; i < 3; i++ {
				if a.deviceManager.SetManualGear(cfg.ManualGear, cfg.ManualLevel) {
					break
				}
				if i < 2 {
					time.Sleep(100 * time.Millisecond)
				}
			}
		}
	}

	if cfg.CustomSpeedEnabled {
		a.deviceManager.SetCustomFanSpeed(cfg.CustomSpeedRPM)
	}

	if cfg.GearLight {
		a.deviceManager.SetGearLight(true)
	}

	if cfg.PowerOnStart {
		a.deviceManager.SetPowerOnStart(true)
	}

	if cfg.SmartStartStop != "" && cfg.SmartStartStop != "off" {
		a.deviceManager.SetSmartStartStop(cfg.SmartStartStop)
	}

	if cfg.Brightness > 0 {
		a.deviceManager.SetBrightness(cfg.Brightness)
	}

	if cfg.RGBConfig != nil {
		params := ipc.SetRGBModeParams{
			Mode:       cfg.RGBConfig.Mode,
			Colors:     make([]ipc.RGBColorParam, len(cfg.RGBConfig.Colors)),
			Speed:      cfg.RGBConfig.Speed,
			Brightness: cfg.RGBConfig.Brightness,
		}
		for i, color := range cfg.RGBConfig.Colors {
			params.Colors[i] = ipc.RGBColorParam{R: color.R, G: color.G, B: color.B}
		}
		a.SetRGBMode(params)
	}

	a.logInfo("配置应用完成")
}

func (a *CoreApp) ConnectDevice() bool {
	a.mutex.Lock()
	a.userDisconnected = false
	a.mutex.Unlock()

	success, deviceInfo := a.deviceManager.Connect()
	if success {
		a.mutex.Lock()
		a.isConnected = true
		a.mutex.Unlock()

		if deviceInfo != nil && a.ipcServer != nil {
			a.ipcServer.BroadcastEvent(ipc.EventDeviceConnected, deviceInfo)
		}

		go a.startTemperatureMonitoring()
		a.applyConfigOnConnect()
	} else if a.ipcServer != nil {
		a.ipcServer.BroadcastEvent(ipc.EventDeviceError, "连接失败")
	}
	return success
}

func (a *CoreApp) DisconnectDevice() {
	a.mutex.Lock()
	a.userDisconnected = true
	if a.monitoringTemp {
		select {
		case a.stopMonitoring <- true:
		default:
		}
		a.monitoringTemp = false
	}
	a.isConnected = false
	a.mutex.Unlock()

	a.deviceManager.Disconnect()
	if a.ipcServer != nil {
		a.ipcServer.BroadcastEvent(ipc.EventDeviceDisconnected, nil)
	}
}

func (a *CoreApp) GetDeviceStatus() map[string]any {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	return map[string]any{
		"connected":   a.isConnected,
		"monitoring":  a.monitoringTemp,
		"currentData": a.deviceManager.GetCurrentFanData(),
		"temperature": a.currentTemp,
	}
}

func (a *CoreApp) UpdateConfig(cfg types.AppConfig) error {
	a.mutex.Lock()
	oldCfg := a.configManager.Get()
	shouldStartMonitor := !a.monitoringTemp && a.isConnected && cfg.AutoControl
	cfg.ConfigPath = oldCfg.ConfigPath
	err := a.configManager.Update(cfg)
	a.mutex.Unlock()
	if shouldStartMonitor {
		go a.startTemperatureMonitoring()
	}
	return err
}

func (a *CoreApp) SetFanCurve(curve []types.FanCurvePoint) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	cfg := a.configManager.Get()
	cfg.FanCurve = curve
	return a.configManager.Update(cfg)
}

func (a *CoreApp) SetAutoControl(enabled bool) error {
	a.mutex.Lock()
	cfg := a.configManager.Get()
	if enabled && cfg.CustomSpeedEnabled {
		a.mutex.Unlock()
		return fmt.Errorf("自定义转速模式下无法开启智能变频")
	}
	cfg.AutoControl = enabled
	if enabled {
		a.userSetAutoControl = true
	}
	shouldStartMonitor := enabled && !a.monitoringTemp && a.isConnected
	a.configManager.Set(cfg)
	err := a.configManager.Save()
	isConnected := a.isConnected
	a.mutex.Unlock()

	// 修复: 在锁外启动 goroutine，避免 startTemperatureMonitoring 锁竞态
	if shouldStartMonitor {
		go a.startTemperatureMonitoring()
	}
	if !enabled && isConnected {
		a.safeGo("applyCurrentGearSetting", func() {
			time.Sleep(200 * time.Millisecond)
			a.applyCurrentGearSetting()
		})
	} else if enabled && isConnected {
		// 当开启智能变频时（从手动模式切换过来），需要恢复RGB状态
		a.safeGo("restoreCurrentRGB-autoControl", func() {
			time.Sleep(300 * time.Millisecond) // 给硬件更多时间切换状态
			a.restoreCurrentRGB()
		})
		// 确保进入自动模式，即使温度监控已经在运行
		a.safeGo("enterAutoMode", func() {
			time.Sleep(100 * time.Millisecond) // 等待一下再进入自动模式
			if err := a.deviceManager.EnterAutoMode(); err != nil {
				a.logError("进入自动模式失败: %v", err)
			}
		})
	}

	if a.ipcServer != nil {
		a.ipcServer.BroadcastEvent(ipc.EventConfigUpdate, cfg)
	}
	return err
}

func (a *CoreApp) applyCurrentGearSetting() {
	fanData := a.deviceManager.GetCurrentFanData()
	if fanData == nil {
		return
	}
	cfg := a.configManager.Get()
	success := a.deviceManager.SetManualGear(fanData.SetGear, cfg.ManualLevel)

	if success && a.isConnected {
		a.safeGo("restoreCurrentRGB-applyGear", func() {
			time.Sleep(200 * time.Millisecond)
			a.restoreCurrentRGB()
		})
	}
}

func (a *CoreApp) SetManualGear(gear, level string) bool {
	cfg := a.configManager.Get()
	cfg.ManualGear = gear
	cfg.ManualLevel = level
	a.configManager.Update(cfg)

	success := a.deviceManager.SetManualGear(gear, level)

	// 当用户主动点击按钮切换到 手动低/中/高时，硬件必定会重置状态
	if success && a.isConnected {
		a.safeGo("restoreCurrentRGB-manualGear", func() {
			time.Sleep(200 * time.Millisecond)
			a.restoreCurrentRGB()
		})
	}
	return success
}

func (a *CoreApp) SetCustomSpeed(enabled bool, rpm int) error {
	a.mutex.Lock()
	cfg := a.configManager.Get()
	if enabled {
		if cfg.AutoControl {
			cfg.AutoControl = false
			if a.monitoringTemp {
				select {
				case a.stopMonitoring <- true:
				default:
				}
			}
		}
		cfg.CustomSpeedEnabled = true
		cfg.CustomSpeedRPM = rpm
	} else {
		cfg.CustomSpeedEnabled = false
	}
	a.configManager.Set(cfg)
	err := a.configManager.Save()
	isConnected := a.isConnected
	a.mutex.Unlock()

	if enabled && isConnected {
		a.safeGo("setCustomFanSpeed", func() {
			a.deviceManager.SetCustomFanSpeed(rpm)
		})
	}

	if a.ipcServer != nil {
		a.ipcServer.BroadcastEvent(ipc.EventConfigUpdate, cfg)
	}

	if isConnected {
		a.safeGo("restoreCurrentRGB-customSpeed", func() {
			time.Sleep(200 * time.Millisecond)
			a.restoreCurrentRGB()
		})
	}

	return err
}

func (a *CoreApp) SetGearLight(enabled bool) bool {
	if !a.deviceManager.SetGearLight(enabled) {
		return false
	}
	cfg := a.configManager.Get()
	cfg.GearLight = enabled
	a.configManager.Update(cfg)
	if a.ipcServer != nil {
		a.ipcServer.BroadcastEvent(ipc.EventConfigUpdate, cfg)
	}
	return true
}

func (a *CoreApp) SetPowerOnStart(enabled bool) bool {
	if !a.deviceManager.SetPowerOnStart(enabled) {
		return false
	}
	cfg := a.configManager.Get()
	cfg.PowerOnStart = enabled
	a.configManager.Update(cfg)
	if a.ipcServer != nil {
		a.ipcServer.BroadcastEvent(ipc.EventConfigUpdate, cfg)
	}
	return true
}

func (a *CoreApp) SetSmartStartStop(mode string) bool {
	if !a.deviceManager.SetSmartStartStop(mode) {
		return false
	}
	cfg := a.configManager.Get()
	cfg.SmartStartStop = mode
	a.configManager.Update(cfg)
	if a.ipcServer != nil {
		a.ipcServer.BroadcastEvent(ipc.EventConfigUpdate, cfg)
	}
	return true
}

func (a *CoreApp) SetBrightness(percentage int) bool {
	if !a.deviceManager.SetBrightness(percentage) {
		return false
	}
	cfg := a.configManager.Get()
	cfg.Brightness = percentage
	a.configManager.Update(cfg)
	if a.ipcServer != nil {
		a.ipcServer.BroadcastEvent(ipc.EventConfigUpdate, cfg)
	}
	return true
}

func (a *CoreApp) SetRGBMode(params ipc.SetRGBModeParams) bool {
	if !a.isConnected {
		return false
	}

	var speed byte
	switch params.Speed {
	case "fast":
		speed = rgb.SpeedFast
	case "slow":
		speed = rgb.SpeedSlow
	default:
		speed = rgb.SpeedMedium
	}
	brightness := byte(params.Brightness)
	toRGBColor := func(c ipc.RGBColorParam) rgb.Color {
		return rgb.Color{R: byte(c.R), G: byte(c.G), B: byte(c.B)}
	}

	var success bool

	// 从deviceManager获取独立的rgbController进行操作
	rgbCtrl := a.deviceManager.RGB()

	switch params.Mode {
	case "smart":
		a.mutex.Lock()
		a.lastSmartModeLevel = 0
		curTemp := a.currentTemp.MaxTemp
		a.mutex.Unlock()

		var level byte = 1
		if curTemp > 0 {
			if curTemp < 60 {
				level = 1
			} else if curTemp < 85 {
				level = 2
			} else if curTemp < 90 {
				level = 3
			} else {
				level = 4
			}
		}

		success = rgbCtrl.SetSmartTempLevel(level)
		if success {
			a.mutex.Lock()
			a.lastSmartModeLevel = level
			a.mutex.Unlock()
		}
	case "off":
		success = rgbCtrl.SetOff()
	case "static_single":
		color := rgb.Color{R: 255, G: 255, B: 255}
		if len(params.Colors) > 0 {
			color = toRGBColor(params.Colors[0])
		}
		success = rgbCtrl.SetStaticSingle(color, brightness)
	case "static_multi":
		var colors [3]rgb.Color
		colors[0] = rgb.Color{R: 255, G: 0, B: 0}
		colors[1] = rgb.Color{R: 0, G: 255, B: 0}
		colors[2] = rgb.Color{R: 0, G: 0, B: 255}
		for i := 0; i < 3 && i < len(params.Colors); i++ {
			colors[i] = toRGBColor(params.Colors[i])
		}
		success = rgbCtrl.SetStaticMulti(colors, brightness)
	case "rotation":
		colors := make([]rgb.Color, 0)
		for _, c := range params.Colors {
			colors = append(colors, toRGBColor(c))
		}
		success = rgbCtrl.SetRotation(colors, speed, brightness)
	case "breathing":
		colors := make([]rgb.Color, 0)
		for _, c := range params.Colors {
			colors = append(colors, toRGBColor(c))
		}
		success = rgbCtrl.SetBreathing(colors, speed, brightness)
	case "flowing":
		success = rgbCtrl.SetFlowing(speed, brightness)
	default:
		return false
	}

	if success {
		cfg := a.configManager.Get()
		rgbColors := make([]types.RGBColorConfig, len(params.Colors))
		for i, c := range params.Colors {
			rgbColors[i] = types.RGBColorConfig{R: c.R, G: c.G, B: c.B}
		}
		cfg.RGBConfig = &types.RGBConfig{
			Mode:       params.Mode,
			Colors:     rgbColors,
			Speed:      params.Speed,
			Brightness: params.Brightness,
		}
		a.configManager.Update(cfg)
		_ = a.configManager.Save()
		if a.ipcServer != nil {
			a.ipcServer.BroadcastEvent(ipc.EventConfigUpdate, cfg)
		}
	}
	return success
}

func (a *CoreApp) GetDebugInfo() map[string]any {
	a.mutex.RLock()
	debugMode := a.debugMode
	isConnected := a.isConnected
	monitoringTemp := a.monitoringTemp
	a.mutex.RUnlock()

	return map[string]any{
		"debugMode":       debugMode,
		"isConnected":     isConnected,
		"guiLastResponse": time.Unix(atomic.LoadInt64(&a.guiLastResponse), 0).Format("2006-01-02 15:04:05"),
		"monitoringTemp":  monitoringTemp,
		"hasGUIClients":   a.ipcServer != nil && a.ipcServer.HasClients(),
	}
}

func (a *CoreApp) SetDebugMode(enabled bool) error {
	a.mutex.Lock()
	cfg := a.configManager.Get()
	cfg.DebugMode = enabled
	a.debugMode = enabled
	if a.logger != nil {
		a.logger.SetDebugMode(enabled)
	}
	a.configManager.Set(cfg)
	err := a.configManager.Save()
	a.mutex.Unlock()
	if err != nil {
		return err
	}
	if a.ipcServer != nil {
		a.ipcServer.BroadcastEvent(ipc.EventConfigUpdate, cfg)
	}
	return nil
}

func (a *CoreApp) startTemperatureMonitoring() {
	a.mutex.Lock()
	if a.monitoringTemp {
		a.mutex.Unlock()
		a.logDebug("温度监控已在运行中，跳过重复启动")
		return
	}
	a.monitoringTemp = true
	isConnected := a.isConnected
	a.mutex.Unlock()

	// 清空 stopMonitoring 中可能残留的信号，
	// 否则新启动的监控goroutine会在第一个select就读到旧信号立即退出
	for len(a.stopMonitoring) > 0 {
		<-a.stopMonitoring
	}

	if isConnected {
		if err := a.deviceManager.EnterAutoMode(); err != nil {
			a.logError("进入自动模式失败: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	cfg := a.configManager.Get()

	intervalSec := cfg.TempUpdateRate
	if intervalSec < 1 {
		intervalSec = 1
	}
	updateInterval := time.Duration(intervalSec) * time.Second
	ticker := time.NewTicker(updateInterval)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				capturePanic(a, "startTemperatureMonitoring", r)
			}
			ticker.Stop()
			a.mutex.Lock()
			a.monitoringTemp = false
			a.mutex.Unlock()
		}()

		sampleCount := max(cfg.TempSampleCount, 1)
		tempSamples := make([]int, 0, sampleCount)
		currentIntervalSec := intervalSec

		for {
			select {
			case <-a.stopMonitoring:
				return

			case <-ticker.C:
				temp := a.tempReader.Read()

				a.mutex.Lock()
				a.currentTemp = temp
				a.mutex.Unlock()

				if a.ipcServer != nil {
					go func(t types.TemperatureData) {
						defer func() { recover() }()
						a.ipcServer.BroadcastEvent(ipc.EventTemperatureUpdate, t)
					}(temp)
				}

				cfg := a.configManager.Get()

				// 分离式 RGB 智能温控判定
				if cfg.RGBConfig != nil && cfg.RGBConfig.Mode == "smart" && temp.MaxTemp > 0 {
					var level byte = 1
					if temp.MaxTemp < 60 {
						level = 1
					} else if temp.MaxTemp < 85 {
						level = 2
					} else if temp.MaxTemp < 90 {
						level = 3
					} else {
						level = 4
					}

					a.mutex.Lock()
					changed := a.lastSmartModeLevel != level
					if changed {
						a.lastSmartModeLevel = level
					}
					a.mutex.Unlock()

					if changed {
						a.deviceManager.RGB().AsyncSetSmartTempLevel(level)
					}
				}

				// 原有的风扇速度控制
				if cfg.AutoControl && temp.MaxTemp > 0 {
					newSampleCount := max(cfg.TempSampleCount, 1)
					if newSampleCount != sampleCount {
						sampleCount = newSampleCount
						tempSamples = make([]int, 0, sampleCount)
					}
					// 动态响应采样间隔配置变更
					newIntervalSec := cfg.TempUpdateRate
					if newIntervalSec < 1 {
						newIntervalSec = 1
					}
					if newIntervalSec != currentIntervalSec {
						currentIntervalSec = newIntervalSec
						ticker.Reset(time.Duration(currentIntervalSec) * time.Second)
					}
					tempSamples = append(tempSamples, temp.MaxTemp)
					if len(tempSamples) > sampleCount {
						tempSamples = tempSamples[len(tempSamples)-sampleCount:]
					}
					avgTemp := 0
					for _, t := range tempSamples {
						avgTemp += t
					}
					avgTemp = avgTemp / len(tempSamples)

					targetRPM := temperature.CalculateTargetRPM(avgTemp, cfg.FanCurve)
					if targetRPM > 0 {
						a.deviceManager.SetFanSpeed(targetRPM)
					}
				}
			}
		}
	}()
}

func (a *CoreApp) startHealthMonitoring() {
	if a.logger != nil {
		go a.logger.CleanOldLogs()
	}

	// 设备健康检查使用指数退避策略
	baseInterval := 5 * time.Second // 基础探测频率：5秒
	maxInterval := 60 * time.Second // 最大探测频率：60秒
	currentInterval := baseInterval

	for {
		select {
		case <-time.After(currentInterval):
			a.checkDeviceHealth(&currentInterval, baseInterval, maxInterval)
		case <-a.cleanupChan:
			return
		}
	}
}

func (a *CoreApp) checkDeviceHealth(currentInterval *time.Duration, baseInterval, maxInterval time.Duration) {
	a.mutex.RLock()
	connected := a.isConnected
	userDid := a.userDisconnected
	a.mutex.RUnlock()

	if !connected {
		if userDid {
			return
		}
		a.logInfo("设备Watchdog: 设备未连接，尝试重新连接")

		// 尝试重连设备
		if a.ConnectDevice() {
			a.logInfo("设备Watchdog: 设备重连成功")
			*currentInterval = baseInterval // 重连成功，重置为基础心跳频率
		} else {
			a.logDebug("设备Watchdog: 重连失败")

			// 指数退避，拉长下次探测的时间
			*currentInterval *= 2
			if *currentInterval > maxInterval {
				*currentInterval = maxInterval
			}
			a.logDebug("设备Watchdog: 下次探测将在 %v 后进行", *currentInterval)
		}
	} else {
		// 连接状态下，检查设备是否真的在线
		if !a.deviceManager.IsConnected() {
			a.logError("设备Watchdog: 检测到设备状态不一致，触发断开回调")
			a.onDeviceDisconnect()
			*currentInterval = baseInterval // 准备立即开始快速重连
		} else {
			// 设备在线，保持正常的心跳频率
			*currentInterval = baseInterval
			a.logDebug("设备Watchdog: 设备连接正常")
		}
	}
}

func (a *CoreApp) cleanup() {
	select {
	case a.cleanupChan <- true:
	default:
	}
	if a.healthCheckTicker != nil {
		a.healthCheckTicker.Stop()
	}
	if a.logger != nil {
		a.logger.Close()
	}
}

func (a *CoreApp) logInfo(format string, v ...any) {
	if a.logger != nil {
		a.logger.Info(format, v...)
	}
}

func (a *CoreApp) logError(format string, v ...any) {
	if a.logger != nil {
		a.logger.Error(format, v...)
	}
}

func (a *CoreApp) logDebug(format string, v ...any) {
	if a.logger != nil {
		a.logger.Debug(format, v...)
	}
}

// restoreCurrentRGB 恢复当前配置的RGB设置
func (a *CoreApp) restoreCurrentRGB() {
	if !a.isConnected {
		return
	}
	cfg := a.configManager.Get()
	if cfg.RGBConfig != nil {
		params := ipc.SetRGBModeParams{
			Mode:       cfg.RGBConfig.Mode,
			Colors:     make([]ipc.RGBColorParam, len(cfg.RGBConfig.Colors)),
			Speed:      cfg.RGBConfig.Speed,
			Brightness: cfg.RGBConfig.Brightness,
		}
		for i, color := range cfg.RGBConfig.Colors {
			params.Colors[i] = ipc.RGBColorParam{R: color.R, G: color.G, B: color.B}
		}
		a.SetRGBMode(params)
	}
}

func (a *CoreApp) RestartService() bool {
	a.logInfo("收到重启服务请求，通过 powershell Restart-Service 触发完整重启")
	const serviceName = "BS2PRO_CoreService"

	go func() {
		psCommand := fmt.Sprintf(`Restart-Service -Name "%s" -Force`, serviceName)
		cmd := exec.Command("powershell",
			"-NonInteractive",
			"-Command", psCommand,
		)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
		}
		if err := cmd.Start(); err != nil {
			a.logError("启动 powershell Restart-Service 失败: %v", err)
			return
		}
	}()

	return true
}

func (a *CoreApp) StopService() bool {
	a.logInfo("收到停止服务请求，通过 powershell Stop-Service 触发停止")
	const serviceName = "BS2PRO_CoreService"

	go func() {
		psCommand := fmt.Sprintf(`Stop-Service -Name "%s" -Force`, serviceName)
		cmd := exec.Command("powershell",
			"-NonInteractive",
			"-Command", psCommand,
		)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
		}
		if err := cmd.Start(); err != nil {
			a.logError("启动 powershell Stop-Service 失败: %v", err)
			return
		}
	}()

	return true
}

// safeGo 安全地启动一个goroutine，自动捕获并报告panic
func (a *CoreApp) safeGo(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				capturePanic(a, "goroutine:"+name, r)
			}
		}()

		fn()
	}()
}
