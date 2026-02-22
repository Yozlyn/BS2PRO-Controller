package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TIANLI0/BS2PRO-Controller/internal/asus"
	"github.com/TIANLI0/BS2PRO-Controller/internal/autostart"
	"github.com/TIANLI0/BS2PRO-Controller/internal/config"
	"github.com/TIANLI0/BS2PRO-Controller/internal/device"
	"github.com/TIANLI0/BS2PRO-Controller/internal/ipc"
	"github.com/TIANLI0/BS2PRO-Controller/internal/logger"
	"github.com/TIANLI0/BS2PRO-Controller/internal/temperature"
	"github.com/TIANLI0/BS2PRO-Controller/internal/types"
	"github.com/TIANLI0/BS2PRO-Controller/internal/version"
)

type CoreApp struct {
	ctx context.Context

	// 管理器
	deviceManager    *device.Manager
	asusClient       *asus.Client
	tempReader       *temperature.Reader
	configManager    *config.Manager
	autostartManager *autostart.Manager
	logger           *logger.CustomLogger
	ipcServer        *ipc.Server

	// 状态
	isConnected        bool
	monitoringTemp     bool
	userDisconnected   bool
	currentTemp        types.TemperatureData
	lastDeviceMode     string
	userSetAutoControl bool
	isAutoStartLaunch  bool
	debugMode          bool

	// 监控相关
	guiLastResponse   int64
	guiMonitorEnabled bool
	healthCheckTicker *time.Ticker
	cleanupChan       chan bool
	quitChan          chan bool

	// 同步
	mutex          sync.RWMutex
	stopMonitoring chan bool
}

// NewCoreApp 创建核心应用实例
func NewCoreApp(debugMode, isAutoStart bool) *CoreApp {
	// 初始化日志系统
	installDir := config.GetInstallDir()
	customLogger, err := logger.NewCustomLogger(debugMode, installDir)
	if err != nil {
		// 如果初始化失败，无法记录，直接退出
		panic(fmt.Sprintf("初始化日志系统失败: %v", err))
	} else {
		customLogger.Info("核心服务启动")
		customLogger.Info("安装目录: %s", installDir)
		customLogger.CleanOldLogs()
	}

	// 创建管理器
	// 初始化 ASUS ACPI 客户端
	asusClient, err := asus.NewClient()
	if err != nil {
		customLogger.Warn("ASUS ACPI 客户端初始化失败: %v", err)
	}

	deviceMgr := device.NewManager(customLogger)
	tempReader := temperature.NewReader(asusClient, customLogger)
	configMgr := config.NewManager(installDir, customLogger)
	autostartMgr := autostart.NewManager(customLogger)

	app := &CoreApp{
		ctx:                context.Background(),
		deviceManager:      deviceMgr,
		asusClient:         asusClient,
		tempReader:         tempReader,
		currentTemp:        types.TemperatureData{BridgeOk: true},
		configManager:      configMgr,
		autostartManager:   autostartMgr,
		logger:             customLogger,
		isConnected:        false,
		monitoringTemp:     false,
		stopMonitoring:     make(chan bool, 1),
		lastDeviceMode:     "",
		userSetAutoControl: false,
		isAutoStartLaunch:  isAutoStart,
		debugMode:          debugMode,
		guiLastResponse:    time.Now().Unix(),
		cleanupChan:        make(chan bool, 1),
		quitChan:           make(chan bool, 1),
		guiMonitorEnabled:  true,
	}
	return app
}

func (a *CoreApp) Start() error {
	a.logInfo("=== BS2PRO 核心服务(Windows Service) 启动 ===")
	a.logInfo("版本: %s", version.Get())

	cfg := a.configManager.Load(a.isAutoStartLaunch)
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

	a.logInfo("启动 IPC 服务器")
	a.ipcServer = ipc.NewServer(a.handleIPCRequest, a.logger)
	if err := a.ipcServer.Start(); err != nil {
		a.logError("启动 IPC 服务器失败: %v", err)
		return err
	}

	if cfg.GuiMonitoring {
		go a.startHealthMonitoring()
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				a.logError("延迟连接设备时发生Panic: %v", r)
			}
		}()
		time.Sleep(1 * time.Second)
		a.ConnectDevice()
	}()

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

	// 启动一个协程：先延迟一秒给前端返回成功的 IPC 响应，然后主动终止进程
	go func() {
		time.Sleep(1 * time.Second)
		a.Stop() // 释放硬件句柄
		a.logInfo("核心服务进程自我终止")
		os.Exit(0) // 正常退出
	}()
}

func (a *CoreApp) handleIPCRequest(req ipc.Request) ipc.Response {
	// 捕获 IPC 路由过程中的致命崩溃
	defer func() {
		if r := recover(); r != nil {
			a.logError("处理 IPC 请求时发生 Panic: %v", r)
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
	case ipc.ReqSetWindowsAutoStart:
		var params ipc.SetBoolParams
		if err := json.Unmarshal(req.Data, &params); err != nil {
			return a.errorResponse("解析参数失败: " + err.Error())
		}
		if err := a.SetWindowsAutoStart(params.Enabled); err != nil {
			return a.errorResponse(err.Error())
		}
		return a.successResponse(true)
	case ipc.ReqCheckWindowsAutoStart:
		enabled := a.autostartManager.CheckWindowsAutoStart()
		return a.dataResponse(enabled)
	case ipc.ReqIsRunningAsAdmin:
		isAdmin := a.autostartManager.IsRunningAsAdmin()
		return a.dataResponse(isAdmin)
	case ipc.ReqGetAutoStartMethod:
		method := a.autostartManager.GetAutoStartMethod()
		return a.dataResponse(method)
	case ipc.ReqSetAutoStartWithMethod:
		var params ipc.SetAutoStartWithMethodParams
		if err := json.Unmarshal(req.Data, &params); err != nil {
			return a.errorResponse("解析参数失败: " + err.Error())
		}
		if err := a.autostartManager.SetAutoStartWithMethod(params.Enable, params.Method); err != nil {
			return a.errorResponse(err.Error())
		}
		return a.successResponse(true)
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
	case ipc.ReqIsAutoStartLaunch:
		return a.dataResponse(a.isAutoStartLaunch)
	case ipc.ReqSetRGBMode:
		var params ipc.SetRGBModeParams
		if err := json.Unmarshal(req.Data, &params); err != nil {
			return a.errorResponse("解析RGB参数失败: " + err.Error())
		}
		success := a.SetRGBMode(params)
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
		if a.ipcServer != nil {
			a.ipcServer.BroadcastEvent(ipc.EventConfigUpdate, cfg)
		}
	} else if fanData.WorkMode == "挡位工作模式" && cfg.AutoControl && a.lastDeviceMode == "自动模式(实时转速)" && !a.userSetAutoControl && cfg.IgnoreDeviceOnReconnect {
		a.logInfo("检测到设备模式变化，但已开启断连保持配置模式，保持APP配置不变")
	}

	a.lastDeviceMode = fanData.WorkMode
	if a.userSetAutoControl {
		a.userSetAutoControl = false
	}
	a.mutex.Unlock()

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
	// 防止重连时发生意外崩溃导致整个协程死掉
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
	cfg := a.configManager.Get()
	if cfg.AutoControl {
		go a.startTemperatureMonitoring()
	} else if cfg.CustomSpeedEnabled {
		if !a.deviceManager.SetCustomFanSpeed(cfg.CustomSpeedRPM) {
			a.logError("重新应用自定义转速失败")
		}
	}
	if cfg.GearLight {
		if !a.deviceManager.SetGearLight(true) {
			a.logError("重新开启挡位灯失败")
		}
	}
	if cfg.PowerOnStart {
		if !a.deviceManager.SetPowerOnStart(true) {
			a.logError("重新开启通电自启动失败")
		}
	}
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

		cfg := a.configManager.Get()
		if cfg.AutoControl {
			go a.startTemperatureMonitoring()
		}
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
	defer a.mutex.Unlock()
	oldCfg := a.configManager.Get()
	if cfg.AutoControl && !a.monitoringTemp && a.isConnected {
		go a.startTemperatureMonitoring()
	} else if !cfg.AutoControl && a.monitoringTemp {
		select {
		case a.stopMonitoring <- true:
		default:
		}
	}
	cfg.ConfigPath = oldCfg.ConfigPath
	return a.configManager.Update(cfg)
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
	defer a.mutex.Unlock()
	cfg := a.configManager.Get()
	if enabled && cfg.CustomSpeedEnabled {
		return fmt.Errorf("自定义转速模式下无法开启智能变频")
	}
	cfg.AutoControl = enabled
	if enabled {
		a.userSetAutoControl = true
	}
	if enabled && !a.monitoringTemp && a.isConnected {
		go a.startTemperatureMonitoring()
	} else if !enabled && a.monitoringTemp {
		select {
		case a.stopMonitoring <- true:
		default:
		}
		if a.isConnected {
			go func() {
				time.Sleep(200 * time.Millisecond)
				a.applyCurrentGearSetting()
			}()
		}
	}
	a.configManager.Set(cfg)
	err := a.configManager.Save()
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
	a.deviceManager.SetManualGear(fanData.SetGear, cfg.ManualLevel)
}

func (a *CoreApp) SetManualGear(gear, level string) bool {
	cfg := a.configManager.Get()
	cfg.ManualGear = gear
	cfg.ManualLevel = level
	a.configManager.Update(cfg)
	return a.deviceManager.SetManualGear(gear, level)
}

func (a *CoreApp) SetCustomSpeed(enabled bool, rpm int) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()
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
		if a.isConnected {
			go a.deviceManager.SetCustomFanSpeed(rpm)
		}
	} else {
		cfg.CustomSpeedEnabled = false
	}
	a.configManager.Set(cfg)
	err := a.configManager.Save()
	if a.ipcServer != nil {
		a.ipcServer.BroadcastEvent(ipc.EventConfigUpdate, cfg)
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
		speed = device.RGBSpeedFast
	case "slow":
		speed = device.RGBSpeedSlow
	default:
		speed = device.RGBSpeedMedium
	}
	brightness := byte(params.Brightness)
	toRGBColor := func(c ipc.RGBColorParam) device.RGBColor {
		return device.RGBColor{R: byte(c.R), G: byte(c.G), B: byte(c.B)}
	}

	var success bool
	switch params.Mode {
	case "smart":
		success = a.deviceManager.SetRGBSmartTemp()
	case "off":
		success = a.deviceManager.SetRGBOff()
	case "static_single":
		color := device.RGBColor{R: 255, G: 255, B: 255}
		if len(params.Colors) > 0 {
			color = toRGBColor(params.Colors[0])
		}
		success = a.deviceManager.SetRGBStaticSingle(color, brightness)
	case "static_multi":
		var colors [3]device.RGBColor
		colors[0] = device.RGBColor{R: 255, G: 0, B: 0}
		colors[1] = device.RGBColor{R: 0, G: 255, B: 0}
		colors[2] = device.RGBColor{R: 0, G: 0, B: 255}
		for i := 0; i < 3 && i < len(params.Colors); i++ {
			colors[i] = toRGBColor(params.Colors[i])
		}
		success = a.deviceManager.SetRGBStaticMulti(colors, brightness)
	case "rotation":
		colors := make([]device.RGBColor, 0)
		for _, c := range params.Colors {
			colors = append(colors, toRGBColor(c))
		}
		if len(colors) == 0 {
			colors = []device.RGBColor{{R: 255, G: 0, B: 0}, {R: 0, G: 255, B: 0}, {R: 0, G: 0, B: 255}}
		}
		success = a.deviceManager.SetRGBRotation(colors, speed, brightness)
	case "breathing":
		colors := make([]device.RGBColor, 0)
		for _, c := range params.Colors {
			colors = append(colors, toRGBColor(c))
		}
		if len(colors) == 0 {
			colors = []device.RGBColor{{R: 0, G: 255, B: 0}}
		}
		success = a.deviceManager.SetRGBBreathing(colors, speed, brightness)
	case "flowing":
		success = a.deviceManager.SetRGBFlowing(speed, brightness)
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

func (a *CoreApp) SetWindowsAutoStart(enable bool) error {
	err := a.autostartManager.SetWindowsAutoStart(enable)
	if err == nil {
		cfg := a.configManager.Get()
		cfg.WindowsAutoStart = enable
		a.configManager.Update(cfg)
		if a.ipcServer != nil {
			a.ipcServer.BroadcastEvent(ipc.EventConfigUpdate, cfg)
		}
	}
	return err
}

func (a *CoreApp) GetDebugInfo() map[string]any {
	return map[string]any{
		"debugMode":       a.debugMode,
		"isConnected":     a.isConnected,
		"guiLastResponse": time.Unix(atomic.LoadInt64(&a.guiLastResponse), 0).Format("2006-01-02 15:04:05"),
		"monitoringTemp":  a.monitoringTemp,
		"autoStartLaunch": a.isAutoStartLaunch,
		"hasGUIClients":   a.ipcServer != nil && a.ipcServer.HasClients(),
	}
}

func (a *CoreApp) SetDebugMode(enabled bool) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	cfg := a.configManager.Get()
	cfg.DebugMode = enabled
	a.debugMode = enabled
	if a.logger != nil {
		a.logger.SetDebugMode(enabled)
	}
	a.configManager.Set(cfg)
	if err := a.configManager.Save(); err != nil {
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
		return
	}
	a.monitoringTemp = true
	a.mutex.Unlock()

	if a.isConnected {
		if err := a.deviceManager.EnterAutoMode(); err != nil {
			a.logError("进入自动模式失败: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	cfg := a.configManager.Get()

	// 防止更新频率设置过低导致 CPU 被打满或底层驱动卡死
	intervalSec := cfg.TempUpdateRate
	if intervalSec < 1 {
		intervalSec = 1 // 强制最低 1 秒钟 1 次
	}
	updateInterval := time.Duration(intervalSec) * time.Second
	ticker := time.NewTicker(updateInterval)

	go func() {
		// 捕获第三方硬件驱动(ASUS WMI/NVML) 内部发生的致命异常
		defer func() {
			if r := recover(); r != nil {
				a.logError("致命错误：温度监控协程崩溃: %v", r)
			}
			ticker.Stop()
			a.mutex.Lock()
			a.monitoringTemp = false
			a.mutex.Unlock()
		}()

		sampleCount := max(cfg.TempSampleCount, 1)
		tempSamples := make([]int, 0, sampleCount)

		for {
			select {
			case <-a.stopMonitoring:
				return // 收到停止信号，安全退出并触发 defer

			case <-ticker.C:
				temp := a.tempReader.Read() // 如果底层驱动崩了，上面的 defer兜底

				a.mutex.Lock()
				a.currentTemp = temp
				a.mutex.Unlock()

				if a.ipcServer != nil {
					a.ipcServer.BroadcastEvent(ipc.EventTemperatureUpdate, temp)
				}

				cfg := a.configManager.Get()
				if cfg.AutoControl && temp.MaxTemp > 0 {
					newSampleCount := max(cfg.TempSampleCount, 1)
					if newSampleCount != sampleCount {
						sampleCount = newSampleCount
						tempSamples = make([]int, 0, sampleCount)
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
	a.healthCheckTicker = time.NewTicker(30 * time.Second)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				a.logError("健康监控协程崩溃: %v", r)
			}
			a.healthCheckTicker.Stop()
		}()
		for {
			select {
			case <-a.healthCheckTicker.C:
				a.performHealthCheck()
			case <-a.cleanupChan:
				return
			}
		}
	}()
	if a.logger != nil {
		go a.logger.CleanOldLogs()
	}
}

func (a *CoreApp) performHealthCheck() {
	a.checkDeviceHealth()
	a.logDebug("健康检查完成 - 设备连接:%v", a.isConnected)
}

func (a *CoreApp) checkDeviceHealth() {
	a.mutex.RLock()
	connected := a.isConnected
	userDid := a.userDisconnected
	a.mutex.RUnlock()

	if !connected {
		if userDid {
			return
		}
		a.logInfo("健康检查: 设备未连接，尝试重新连接")
		go func() {
			if a.ConnectDevice() {
				a.logInfo("健康检查: 设备重连成功")
			}
		}()
	} else {
		if !a.deviceManager.IsConnected() {
			a.logError("健康检查: 检测到设备状态不一致，触发断开回调")
			a.onDeviceDisconnect()
		}
	}
}

func (a *CoreApp) cleanup() {
	if a.healthCheckTicker != nil {
		a.healthCheckTicker.Stop()
	}
	select {
	case a.cleanupChan <- true:
	default:
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
