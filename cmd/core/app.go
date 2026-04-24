package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TIANLI0/BS2PRO-Controller/internal/asus"
	"github.com/TIANLI0/BS2PRO-Controller/internal/config"
	"github.com/TIANLI0/BS2PRO-Controller/internal/device"
	"github.com/TIANLI0/BS2PRO-Controller/internal/fanoffset"
	"github.com/TIANLI0/BS2PRO-Controller/internal/ipc"
	"github.com/TIANLI0/BS2PRO-Controller/internal/logger"
	"github.com/TIANLI0/BS2PRO-Controller/internal/notification"
	"github.com/TIANLI0/BS2PRO-Controller/internal/procswitch"
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
	deviceModel        string
	deviceProductID    string
	monitoringTemp     bool
	userDisconnected   bool
	corePaused         bool
	currentTemp        types.TemperatureData
	lastDeviceMode     string
	userSetAutoControl bool
	debugMode          bool

	cleanupChan chan bool

	mutex          sync.RWMutex
	stopMonitoring chan bool

	// 记录当前已经下发的 RGB 智能温度档位
	lastSmartModeLevel byte

	// 重连代数：每次发起新的重连任务时递增，用于取消过期的重连协程
	reconnectGen int32

	// 自动偏移控制器
	fanOffsetCtrl *fanoffset.Controller
	// 风扇命令整形器
	fanCommandShaper *fanCommandShaper
	// 运行时风扇曲线（含自动偏移学习结果，不直接落盘）
	runtimeFanCurve []types.FanCurvePoint

	// 进程联动风扇配置
	processSwitcher               *procswitch.Switcher
	processSwitchStop             chan struct{}
	processSwitchWG               sync.WaitGroup
	activeProcessProfilePath      string
	baseFanCurveBeforeSwitch      []types.FanCurvePoint
	baseOffsetEnabledBeforeSwitch *bool
	lastReportedForegroundProcess string
	notificationManager           *notification.Manager
	systemHotkeyConflicts         []types.HotkeyConflict
}

func NewCoreApp(debugMode bool) *CoreApp {
	installDir := config.GetInstallDir()
	// 日志统一写入 ProgramData\BS2PRO-Controller\logs，与 GUI 进程保持一致
	logBaseDir := filepath.Dir(config.GetLogDir()) // ProgramData\BS2PRO-Controller
	customLogger, err := logger.NewCustomLogger(debugMode, logBaseDir, "core")
	if err != nil {
		// 降级：尝试系统临时目录，避免panic导致崩溃报告无法写入
		fallbackDir := os.TempDir()
		customLogger, err = logger.NewCustomLogger(debugMode, fallbackDir, "core")
		if err != nil {
			// 最坏情况：创建一个只写stderr的logger，保证后续代码不会nil panic
			customLogger, _ = logger.NewCustomLogger(debugMode, ".", "core")
		}
		if customLogger != nil {
			customLogger.Warn("日志目录初始化失败，已降级到临时目录", "fallback_dir", fallbackDir)
		}
	} else {
		customLogger.CleanOldLogs()
	}

	asusClient, err := asus.NewClient()
	if err != nil {
		customLogger.Warn("ASUS ACPI 客户端初始化失败", "error", err)
	}

	deviceMgr := device.NewManager(customLogger)
	tempReader := temperature.NewReader(asusClient, customLogger)
	configMgr := config.NewManager(installDir, customLogger)
	procSwitcher := procswitch.New(configMgr.GetDefaultConfigDir(), customLogger)

	fanOffsetCtrl := fanoffset.New(fanoffset.DefaultConfig(), customLogger)
	if err := fanOffsetCtrl.EnablePersistence(filepath.Join(configMgr.GetStateDir(), "fanoffset")); err != nil {
		customLogger.Warn("初始化风扇长期记忆持久化失败", "error", err)
	}
	commandShaper := newFanCommandShaper()

	app := &CoreApp{
		ctx:                context.Background(),
		deviceManager:      deviceMgr,
		asusClient:         asusClient,
		tempReader:         tempReader,
		currentTemp:        types.TemperatureData{BridgeOk: true},
		configManager:      configMgr,
		logger:             customLogger,
		isConnected:        false,
		deviceModel:        "BS2PRO",
		deviceProductID:    "",
		monitoringTemp:     false,
		stopMonitoring:     make(chan bool, 1),
		lastDeviceMode:     "",
		userSetAutoControl: false,
		debugMode:          debugMode,
		fanOffsetCtrl:      fanOffsetCtrl,
		fanCommandShaper:   commandShaper,
		processSwitcher:    procSwitcher,
		processSwitchStop:  make(chan struct{}),
		cleanupChan:        make(chan bool, 1),
		lastSmartModeLevel: 0,
	}
	app.notificationManager = notification.NewManager(func() bool {
		return app.configManager.Get().NotificationsEnabled
	}, app.emitNotification)
	return app
}

func (a *CoreApp) Start() error {
	a.logInfo("核心服务启动", "version", version.Get(), "install_dir", config.GetInstallDir())

	cfg := a.configManager.Load(false)
	if cfg.DebugMode {
		a.debugMode = true
		if a.logger != nil {
			a.logger.SetDebugMode(true)
		}
	}

	if err := a.deviceManager.Init(); err != nil {
		a.logError("初始化 HID 库失败", "error", err)
		return err
	}
	a.deviceManager.SetCallbacks(a.onFanDataUpdate, a.onDeviceDisconnect)

	a.logInfo("启动 IPC 服务器 (Named Pipe)")
	a.ipcServer = ipc.NewServer(a.handleIPCRequest, a.logger)
	if err := a.ipcServer.Start(); err != nil {
		a.logError("启动 IPC 服务器失败", "pipe", ipc.PipePath, "error", err)
		return err
	}

	if cfg.GuiMonitoring {
		a.logInfo("启动健康监控")
		a.safeGo("startHealthMonitoring", func() {
			a.startHealthMonitoring()
		})
	}

	a.startProcessSwitchMonitoring()

	a.safeGo("delayedConnectDevice", func() {
		time.Sleep(1 * time.Second)
		if !a.ConnectDevice() {
			a.logInfo("初始化连接失败，进入自动重连模式")
			gen := atomic.AddInt32(&a.reconnectGen, 1)
			go a.scheduleReconnect(gen)
		}
	})

	return nil
}

func (a *CoreApp) Stop() {
	a.logInfo("核心服务正在停止...")
	a.logInfo("Stop路径: cleanup -> DisconnectDevice -> CloseIPC")
	close(a.processSwitchStop)
	a.processSwitchWG.Wait()
	if a.fanOffsetCtrl != nil {
		if err := a.fanOffsetCtrl.Close(); err != nil {
			a.logError("停止前刷新风扇长期记忆失败", "error", err)
		}
	}
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

func (a *CoreApp) onQuitRequest() {
	a.logInfo("收到前端的彻底退出请求，准备关闭核心服务...")
	if a.ipcServer != nil {
		a.ipcServer.BroadcastEvent("quit", nil)
	}

	go func() {
		defer func() { _ = recover() }()
		time.Sleep(1 * time.Second)
		a.Stop() // 释放硬件句柄
		a.logInfo("核心服务进程自我终止")
		os.Exit(0) // 正常退出
	}()
}

func (a *CoreApp) handleIPCRequest(req ipc.Request) (res ipc.Response) {
	defer func() {
		if r := recover(); r != nil {
			a.logError("处理 IPC 请求时发生致命异常", "event", req.Type, "error", r)
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
		cfg := a.configWithRuntimeCurve(a.configManager.Get())
		return a.dataResponse(cfg)
	case ipc.ReqRegisterClient:
		return a.successResponse(true)
	case ipc.ReqUpdateConfig:
		var cfg types.AppConfig
		if err := json.Unmarshal(req.Data, &cfg); err != nil {
			return a.errorResponse("解析配置失败: " + err.Error())
		}
		if conflicts := types.DetectHotkeyConflicts(cfg.Hotkeys); len(conflicts) > 0 {
			return a.errorResponse("快捷键存在冲突")
		}
		if err := a.UpdateConfig(cfg); err != nil {
			return a.errorResponse(err.Error())
		}
		return a.successResponse(true)
	case ipc.ReqReportHotkeyConflicts:
		var conflicts []types.HotkeyConflict
		if err := json.Unmarshal(req.Data, &conflicts); err != nil {
			return a.errorResponse("解析快捷键冲突失败: " + err.Error())
		}
		a.setSystemHotkeyConflicts(conflicts)
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
		curve := a.visibleFanCurve(a.configManager.Get().FanCurve)
		return a.dataResponse(curve)
	case ipc.ReqCheckProcessSwitchNow:
		switched := a.CheckProcessSwitchNow()
		return a.successResponse(switched)
	case ipc.ReqReportForegroundProcess:
		var params ipc.ReportForegroundProcessParams
		if err := json.Unmarshal(req.Data, &params); err != nil {
			return a.errorResponse("解析前台进程失败: " + err.Error())
		}
		a.handleForegroundProcessReport(params.ProcessName)
		return a.successResponse(true)
	case ipc.ReqNotifyImportCompleted:
		var params ipc.NotifyProfilesParams
		if err := json.Unmarshal(req.Data, &params); err != nil {
			return a.errorResponse("解析导入通知失败: " + err.Error())
		}
		a.logInfo("收到导入完成通知请求", "event", req.Type, "profile_count", params.ProfileCount)
		if a.notificationManager != nil {
			a.notificationManager.OnConfigImportCompleted(params.ProfileCount)
		}
		return a.successResponse(true)
	case ipc.ReqNotifyExportCompleted:
		var params ipc.NotifyProfilesParams
		if err := json.Unmarshal(req.Data, &params); err != nil {
			return a.errorResponse("解析导出通知失败: " + err.Error())
		}
		a.logInfo("收到导出完成通知请求", "event", req.Type, "profile_count", params.ProfileCount)
		if a.notificationManager != nil {
			a.notificationManager.OnConfigExportCompleted(params.ProfileCount)
		}
		return a.successResponse(true)
	case ipc.ReqApplyOffsetToCurve:
		if err := a.ApplyOffsetToCurve(); err != nil {
			return a.errorResponse(err.Error())
		}
		return a.successResponse(true)
	case ipc.ReqSetAutoControl:
		var params ipc.SetAutoControlParams
		if err := json.Unmarshal(req.Data, &params); err != nil {
			return a.errorResponse("解析参数失败: " + err.Error())
		}
		if err := a.SetAutoControl(params.Enabled); err != nil {
			return a.errorResponse(err.Error())
		}
		return a.successResponse(true)
	case ipc.ReqToggleAutoControl:
		cfg := a.configManager.Get()
		a.logInfo("收到快捷键切换智能变频请求", "event", req.Type, "current", cfg.AutoControl, "next", !cfg.AutoControl)
		if err := a.SetAutoControl(!cfg.AutoControl); err != nil {
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
	case ipc.ReqPing:
		return a.dataResponse("pong")
	case ipc.ReqSetRGBMode:
		var params ipc.SetRGBModeParams
		if err := json.Unmarshal(req.Data, &params); err != nil {
			return a.errorResponse("解析RGB参数失败: " + err.Error())
		}
		success := a.SetRGBMode(params)
		return a.successResponse(success)
	case ipc.ReqToggleProcessSwitch:
		cfg := a.configManager.Get()
		next := !cfg.ProcessSwitchEnabled
		a.logInfo("收到快捷键切换进程联动请求", "event", req.Type, "current", cfg.ProcessSwitchEnabled, "next", next)
		if err := a.SetProcessSwitchEnabled(next); err != nil {
			return a.errorResponse(err.Error())
		}
		return a.successResponse(true)
	case ipc.ReqCycleRGBMode:
		a.logInfo("收到快捷键切换RGB模式请求")
		success := a.CycleRGBMode()
		return a.successResponse(success)
	case ipc.ReqTriggerHotkeyAction:
		var params ipc.TriggerHotkeyActionParams
		if err := json.Unmarshal(req.Data, &params); err != nil {
			return a.errorResponse("解析快捷键动作失败: " + err.Error())
		}
		a.triggerHotkeyAction(params.Action)
		return a.successResponse(true)
	case ipc.ReqSetHotkeyEditMode:
		var params ipc.SetBoolParams
		if err := json.Unmarshal(req.Data, &params); err != nil {
			return a.errorResponse("解析快捷键编辑状态失败: " + err.Error())
		}
		if a.ipcServer != nil {
			a.ipcServer.BroadcastEventToRole(ipc.RoleMonitorAgent, ipc.EventHotkeyEditMode, params)
		}
		return a.successResponse(true)
	case ipc.ReqRestartService:
		a.RestartService()
		return a.successResponse(true)
	case ipc.ReqStopService:
		a.StopService()
		return a.successResponse(true)
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
	if fanData.WorkMode == "挡位工作模式" && cfg.AutoControl && a.lastDeviceMode == "自动模式(实时转速)" && !a.userSetAutoControl && !cfg.IgnoreDeviceOnReconnect && !a.monitoringTemp {
		a.logInfo("检测到设备从自动模式切换到挡位工作模式，自动关闭智能变频", "previous_mode", a.lastDeviceMode, "mode", fanData.WorkMode)
		cfg.AutoControl = false
		if err := a.configManager.Update(cfg); err != nil {
			a.logError("保存配置失败", "error", err)
		}
		shouldBroadcastConfig = true
		broadcastCfg = cfg
	} else if fanData.WorkMode == "挡位工作模式" && cfg.AutoControl && a.lastDeviceMode == "自动模式(实时转速)" && !a.userSetAutoControl && !cfg.IgnoreDeviceOnReconnect && a.monitoringTemp {
		a.logDebug("智能变频监控期间设备临时进入挡位模式")
	} else if fanData.WorkMode == "挡位工作模式" && cfg.AutoControl && a.lastDeviceMode == "自动模式(实时转速)" && !a.userSetAutoControl && cfg.IgnoreDeviceOnReconnect {
		a.logInfo("检测到设备模式变化，但已开启断连保持配置模式，保持APP配置不变")
	}

	a.lastDeviceMode = fanData.WorkMode
	if a.userSetAutoControl {
		a.userSetAutoControl = false
	}
	a.mutex.Unlock()

	// 在锁外进行广播，避免持锁期间阻塞
	if shouldBroadcastConfig {
		a.broadcastConfigUpdate(broadcastCfg)
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
	// 重置设备模式记录，防止重连窗口期内 onFanDataUpdate 用陈旧的 lastDeviceMode 错误判断
	a.lastDeviceMode = ""
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
	if wasConnected && !userDid && a.notificationManager != nil {
		a.notificationManager.OnDeviceDisconnected()
	}

	if !userDid {
		gen := atomic.AddInt32(&a.reconnectGen, 1)
		go a.scheduleReconnect(gen)
	}
}

func (a *CoreApp) scheduleReconnect(gen int32) {
	defer func() {
		if r := recover(); r != nil {
			a.logError("自动重连发生 panic", "error", r, "attempt", gen)
		}
	}()

	// isCancelled 检查本次重连是否应当放弃
	isCancelled := func() bool {
		if atomic.LoadInt32(&a.reconnectGen) != gen {
			return true
		}
		a.mutex.RLock()
		defer a.mutex.RUnlock()
		return a.isConnected || a.userDisconnected
	}

	tryConnect := func() bool {
		if isCancelled() {
			return false
		}
		if a.ConnectDevice() {
			a.logInfo("设备重连成功")
			return true
		}
		return false
	}

	time.Sleep(2 * time.Second)
	if isCancelled() {
		return
	}
	a.logInfo("尝试快速重连...")
	if tryConnect() {
		return
	}

	a.logInfo("快速重连失败，进入等待模式：将在检测到设备接入后自动重连")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if isCancelled() {
			a.logInfo("重连任务已取消")
			return
		}
		if !a.deviceManager.IsDevicePresent() {
			// 设备尚未出现，继续等待，不发起连接
			continue
		}
		a.logInfo("检测到设备接入，尝试重连...")
		if tryConnect() {
			return
		}
		a.logError("设备已出现但连接失败，继续等待...")
	}
}

func (a *CoreApp) applyConfigOnConnect() {
	cfg := a.configManager.Get()
	a.logDebug("开始应用配置到设备")
	a.logDebug("连接后配置摘要", "auto_control", cfg.AutoControl, "custom_speed_enabled", cfg.CustomSpeedEnabled, "custom_speed_rpm", cfg.CustomSpeedRPM)

	time.Sleep(200 * time.Millisecond)

	if !cfg.AutoControl {
		if cfg.ManualGear != "" && cfg.ManualLevel != "" {
			manualApplied := false
			for i := 0; i < 3; i++ {
				if a.deviceManager.SetManualGear(cfg.ManualGear, cfg.ManualLevel) {
					manualApplied = true
					break
				}
				if i < 2 {
					time.Sleep(100 * time.Millisecond)
				}
			}
			if !manualApplied {
				a.logError("连接后恢复手动挡失败", "gear", cfg.ManualGear, "level", cfg.ManualLevel, "attempts", 3)
			}
		}
	}

	if cfg.CustomSpeedEnabled {
		a.logDebug("连接后应用自定义转速", "rpm", cfg.CustomSpeedRPM)
		if !a.deviceManager.SetCustomFanSpeed(cfg.CustomSpeedRPM) {
			a.logError("连接后应用自定义转速失败", "rpm", cfg.CustomSpeedRPM)
		}
	}

	if cfg.GearLight {
		if !a.deviceManager.SetGearLight(true) {
			a.logError("连接后恢复挡位灯失败", "enabled", true)
		}
	}

	if cfg.PowerOnStart {
		if !a.deviceManager.SetPowerOnStart(true) {
			a.logError("连接后恢复开机自启设置失败", "enabled", true)
		}
	}

	if cfg.SmartStartStop != "" && cfg.SmartStartStop != "off" {
		if !a.deviceManager.SetSmartStartStop(cfg.SmartStartStop) {
			a.logError("连接后恢复智能启停设置失败", "mode", cfg.SmartStartStop)
		}
	}

	if cfg.Brightness > 0 {
		if !a.deviceManager.SetBrightness(cfg.Brightness) {
			a.logError("连接后恢复亮度失败", "brightness", cfg.Brightness)
		}
	}

	rgbParams := rgbParamsFromConfig(cfg)
	if !a.applyRGBMode(rgbParams, false) {
		a.logError("连接后恢复 RGB 失败", "mode", rgbParams.Mode, "colors", len(rgbParams.Colors), "speed", rgbParams.Speed, "brightness", rgbParams.Brightness)
	}

	a.logDebug("配置应用完成")
}

func (a *CoreApp) ConnectDevice() bool {
	a.mutex.RLock()
	paused := a.corePaused
	wasDisconnected := !a.isConnected
	a.mutex.RUnlock()
	if paused {
		a.logInfo("核心处于暂停状态，跳过设备连接")
		return false
	}
	a.mutex.Lock()
	a.userDisconnected = false
	a.mutex.Unlock()

	success, deviceInfo := a.deviceManager.Connect()
	if success {
		a.mutex.Lock()
		a.isConnected = true
		a.deviceModel = "BS2PRO"
		a.deviceProductID = ""
		if deviceInfo != nil {
			if m, ok := deviceInfo["model"]; ok && strings.TrimSpace(m) != "" {
				a.deviceModel = m
			}
			if pid, ok := deviceInfo["productId"]; ok {
				a.deviceProductID = pid
			}
		}
		a.mutex.Unlock()

		// 重置风扇运行态控制器
		a.resetFanRuntimeState()

		if deviceInfo != nil && a.ipcServer != nil {
			a.ipcServer.BroadcastEvent(ipc.EventDeviceConnected, deviceInfo)
		}
		if wasDisconnected && a.notificationManager != nil {
			a.notificationManager.OnDeviceReconnected()
		}

		go a.startTemperatureMonitoring()
		a.applyConfigOnConnect()
	} else if a.ipcServer != nil {
		a.ipcServer.BroadcastEvent(ipc.EventDeviceError, "连接失败")
	}
	return success
}

func (a *CoreApp) DisconnectDevice() {
	// 取消任何正在等待的重连协程
	atomic.AddInt32(&a.reconnectGen, 1)

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
	a.deviceModel = "BS2PRO"
	a.deviceProductID = ""
	a.lastDeviceMode = ""
	a.mutex.Unlock()

	// 重置风扇运行态控制器
	a.resetFanRuntimeState()
	if fd := a.deviceManager.GetCurrentFanData(); fd != nil {
		a.logInfo("DisconnectDevice 前设备状态", "mode", fd.WorkMode, "current_rpm", fd.CurrentRPM, "target_rpm", fd.TargetRPM)
	} else {
		a.logInfo("DisconnectDevice 前设备状态: 无风扇数据")
	}

	a.deviceManager.Disconnect()
	if a.ipcServer != nil {
		a.ipcServer.BroadcastEvent(ipc.EventDeviceDisconnected, nil)
	}
	if a.notificationManager != nil {
		a.notificationManager.ResetDeviceDisconnectState()
	}
}

func (a *CoreApp) GetDeviceStatus() map[string]any {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	return map[string]any{
		"connected":   a.isConnected,
		"paused":      a.corePaused,
		"model":       a.deviceModel,
		"productId":   a.deviceProductID,
		"monitoring":  a.monitoringTemp,
		"currentData": a.deviceManager.GetCurrentFanData(),
		"temperature": a.currentTemp,
	}
}

func (a *CoreApp) UpdateConfig(cfg types.AppConfig) error {
	a.mutex.Lock()
	oldCfg := a.configManager.Get()
	if len(a.runtimeFanCurve) > 0 && fanCurvesEqual(cfg.FanCurve, a.runtimeFanCurve) {
		cfg.FanCurve = cloneFanCurvePoints(oldCfg.FanCurve)
	}
	shouldStartMonitor := !a.monitoringTemp && a.isConnected && cfg.AutoControl
	curveChanged := !fanCurvesEqual(oldCfg.FanCurve, cfg.FanCurve)
	offsetChanged := oldCfg.FanCurveOffsetEnabled != cfg.FanCurveOffsetEnabled
	cfg.ConfigPath = oldCfg.ConfigPath
	cfg.HotkeyConflicts = append([]types.HotkeyConflict{}, a.systemHotkeyConflicts...)
	err := a.configManager.Update(cfg)
	a.mutex.Unlock()
	if curveChanged {
		a.clearRuntimeFanCurve()
		a.fanOffsetCtrl.ResetForCurve(cfg.FanCurve, "update_config")
		a.resetFanCommandShaper()
	} else if offsetChanged {
		a.resetFanRuntimeState()
	}
	a.setSystemHotkeyConflicts(nil)
	a.broadcastConfigUpdate(cfg)
	if shouldStartMonitor {
		go a.startTemperatureMonitoring()
	}
	return err
}

func (a *CoreApp) SetProcessSwitchEnabled(enabled bool) error {
	a.mutex.Lock()
	cfg := a.configManager.Get()
	cfg.ProcessSwitchEnabled = enabled
	err := a.configManager.Update(cfg)
	a.mutex.Unlock()
	if err != nil {
		return err
	}
	if enabled {
		a.safeGo("processSwitchImmediateCheck", func() {
			a.runProcessSwitchCheck(a.configManager.Get())
		})
	} else {
		a.restoreBaseFanCurveAfterProcessSwitch()
		a.mutex.Lock()
		a.activeProcessProfilePath = ""
		a.lastReportedForegroundProcess = ""
		a.mutex.Unlock()
	}
	if a.notificationManager != nil {
		a.notificationManager.OnProcessSwitchChanged(enabled)
	}
	a.broadcastConfigUpdate(cfg)
	return nil
}

func (a *CoreApp) SetFanCurve(curve []types.FanCurvePoint) error {
	a.mutex.Lock()
	cfg := a.configManager.Get()
	cfg.FanCurve = cloneFanCurvePoints(curve)
	for i := range cfg.FanCurve {
		cfg.FanCurve[i].Offset = 0
	}
	err := a.configManager.Update(cfg)
	a.mutex.Unlock()
	if err != nil {
		return err
	}
	a.clearRuntimeFanCurve()
	a.fanOffsetCtrl.ResetForCurve(cfg.FanCurve, "set_fan_curve")
	a.resetFanCommandShaper()
	return nil
}

// ApplyOffsetToCurve 将当前偏移量烘焙进基线 RPM，偏移归零，重置偏移控制器
func (a *CoreApp) ApplyOffsetToCurve() error {
	a.mutex.Lock()
	cfg := a.configManager.Get()
	if len(a.runtimeFanCurve) > 0 {
		cfg.FanCurve = cloneFanCurvePoints(a.runtimeFanCurve)
	} else {
		cfg.FanCurve = cloneFanCurvePoints(cfg.FanCurve)
	}
	const minRPM, maxRPM = 500, 4000
	for i := range cfg.FanCurve {
		p := &cfg.FanCurve[i]
		newRPM := p.RPM + p.Offset
		if newRPM < minRPM {
			newRPM = minRPM
		}
		if newRPM > maxRPM {
			newRPM = maxRPM
		}
		p.RPM = newRPM
		p.Offset = 0
	}
	err := a.configManager.Update(cfg)
	a.mutex.Unlock()
	if err != nil {
		return err
	}
	a.clearRuntimeFanCurve()
	a.fanOffsetCtrl.ResetForCurve(cfg.FanCurve, "apply_offset_to_curve")
	a.resetFanCommandShaper()
	a.broadcastConfigUpdate(cfg)
	return nil
}

func (a *CoreApp) startProcessSwitchMonitoring() {
	a.processSwitchWG.Add(1)
	a.safeGo("processSwitchMonitoring", func() {
		defer a.processSwitchWG.Done()
		for {
			a.mutex.RLock()
			paused := a.corePaused
			a.mutex.RUnlock()
			cfg := a.configManager.Get()
			interval := cfg.ProcessSwitchInterval
			if interval < 1 {
				interval = 3
			}

			if !paused && cfg.ProcessSwitchEnabled {
				a.runProcessSwitchCheck(cfg)
			}

			select {
			case <-a.processSwitchStop:
				return
			case <-time.After(time.Duration(interval) * time.Second):
			}
		}
	})
}

func (a *CoreApp) handleForegroundProcessReport(processName string) {
	if !a.configManager.Get().ProcessSwitchEnabled {
		return
	}
	a.mutex.Lock()
	a.lastReportedForegroundProcess = strings.ToLower(strings.TrimSpace(processName))
	a.mutex.Unlock()
	a.logDebug("进程联动收到探针上报前台进程", "process", processName)
}

func (a *CoreApp) emitNotification(req notification.Request) {
	if a.ipcServer == nil || !a.ipcServer.HasRoleClient(ipc.RoleMonitorAgent) {
		a.logDebug("通知未投递，monitor 未连接", "role", ipc.RoleMonitorAgent, "type", req.Type)
		return
	}
	a.logInfo("投递通知到 monitor", "role", ipc.RoleMonitorAgent, "type", req.Type, "title", req.Title)
	a.ipcServer.BroadcastEventToRole(ipc.RoleMonitorAgent, ipc.EventNotificationRequest, req)
}

func (a *CoreApp) runProcessSwitchCheck(cfg types.AppConfig) {
	if a.processSwitcher == nil {
		return
	}

	a.mutex.RLock()
	foregroundProcess := a.lastReportedForegroundProcess
	a.mutex.RUnlock()
	procs := a.processSwitcher.ListProcessNames(foregroundProcess)
	a.logDebug("进程联动当前前台进程集合", "processes", procs)

	rule := a.processSwitcher.MatchRule(cfg.ProcessSwitchRules, procs)
	if rule == nil {
		a.logDebug("进程联动未命中，准备恢复原始风扇曲线")
		a.restoreBaseFanCurveAfterProcessSwitch()
		a.mutex.Lock()
		a.activeProcessProfilePath = ""
		a.mutex.Unlock()
		return
	}

	resolved := a.processSwitcher.ResolveProfilePath(rule.ProfilePath)
	a.mutex.RLock()
	alreadyApplied := strings.EqualFold(a.activeProcessProfilePath, resolved)
	a.mutex.RUnlock()
	if alreadyApplied {
		a.logDebug("进程联动命中但已应用，无需重复切换", "process", rule.ProcessName, "path", resolved)
		return
	}

	curve, err := a.processSwitcher.LoadCurve(rule.ProfilePath)
	if err != nil {
		a.logError("进程联动加载配置失败", "process", rule.ProcessName, "path", rule.ProfilePath, "error", err)
		return
	}

	if err := a.applyProcessCurve(curve, resolved); err != nil {
		a.logError("进程联动应用配置失败", "process", rule.ProcessName, "path", rule.ProfilePath, "error", err)
		return
	}

	a.logInfo("进程联动已切换风扇配置", "process", rule.ProcessName, "path", resolved)
}

func (a *CoreApp) applyProcessCurve(curve []types.FanCurvePoint, profilePath string) error {
	a.mutex.Lock()
	cfg := a.configManager.Get()
	if a.activeProcessProfilePath == "" {
		a.baseFanCurveBeforeSwitch = cloneFanCurvePoints(cfg.FanCurve)
		baseOffsetEnabled := cfg.FanCurveOffsetEnabled
		a.baseOffsetEnabledBeforeSwitch = &baseOffsetEnabled
	}
	cfg.FanCurve = cloneFanCurvePoints(curve)
	for i := range cfg.FanCurve {
		cfg.FanCurve[i].Offset = 0
	}
	cfg.FanCurveOffsetEnabled = false
	err := a.configManager.Update(cfg)
	isConnected := a.isConnected
	currentTemp := a.currentTemp.MaxTemp
	if err == nil {
		a.activeProcessProfilePath = profilePath
	}
	a.mutex.Unlock()
	if err != nil {
		return err
	}
	a.resetFanRuntimeState()

	if isConnected && cfg.AutoControl && currentTemp > 0 {
		thermalTargetRPM := temperature.CalculateTargetRPM(currentTemp, cfg.FanCurve)
		plan := a.planFanCommandTarget(time.Now(), thermalTargetRPM, currentTemp, a.deviceManager.GetCurrentFanData())
		if plan.ShouldSend && plan.CommandTargetRPM > 0 {
			if !a.deviceManager.SetFanSpeed(plan.CommandTargetRPM) {
				a.logError("进程联动应用目标转速失败", "thermal_target_rpm", plan.ThermalTargetRPM, "target_rpm", plan.CommandTargetRPM, "shape_reason", plan.ShapeReason, "temp", currentTemp, "profile", profilePath)
			}
		}
	}

	a.broadcastConfigUpdate(cfg)
	return nil
}

func (a *CoreApp) restoreBaseFanCurveAfterProcessSwitch() {
	a.mutex.Lock()
	if a.activeProcessProfilePath == "" || len(a.baseFanCurveBeforeSwitch) == 0 {
		a.mutex.Unlock()
		return
	}
	a.logDebug("进程联动退出前台，恢复原始风扇曲线", "previous", a.activeProcessProfilePath)
	cfg := a.configManager.Get()
	cfg.FanCurve = cloneFanCurvePoints(a.baseFanCurveBeforeSwitch)
	if a.baseOffsetEnabledBeforeSwitch != nil {
		cfg.FanCurveOffsetEnabled = *a.baseOffsetEnabledBeforeSwitch
	}
	err := a.configManager.Update(cfg)
	if err == nil {
		a.logDebug("进程联动恢复完成", "restored_curve_points", len(cfg.FanCurve), "restored_offset_enabled", cfg.FanCurveOffsetEnabled)
		a.baseFanCurveBeforeSwitch = nil
		a.baseOffsetEnabledBeforeSwitch = nil
	}
	a.mutex.Unlock()
	if err != nil {
		a.logError("恢复原始风扇曲线失败", "error", err)
		return
	}
	a.resetFanRuntimeState()
	a.broadcastConfigUpdate(cfg)
}

func cloneFanCurvePoints(src []types.FanCurvePoint) []types.FanCurvePoint {
	if len(src) == 0 {
		return nil
	}
	dst := make([]types.FanCurvePoint, len(src))
	copy(dst, src)
	return dst
}

func (a *CoreApp) CheckProcessSwitchNow() bool {
	cfg := a.configManager.Get()
	if !cfg.ProcessSwitchEnabled {
		return false
	}
	a.runProcessSwitchCheck(cfg)
	return true
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
	isConnected := a.isConnected
	err := a.configManager.Update(cfg)
	a.mutex.Unlock()
	if err != nil {
		return err
	}
	a.resetFanRuntimeState()

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
		// 当开启智能变频时，仅在智能灯光模式下静默补应用一次智能灯光
		a.safeGo("restoreSmartRGB-autoControl", func() {
			time.Sleep(300 * time.Millisecond) // 给硬件更多时间切换状态
			a.restoreSmartRGBForAutoControl()
		})
		// 确保进入自动模式，即使温度监控已经在运行
		a.safeGo("enterAutoMode", func() {
			time.Sleep(100 * time.Millisecond) // 等待一下再进入自动模式
			if err := a.deviceManager.EnterAutoMode(); err != nil {
				a.logError("进入自动模式失败", "error", err)
			}
		})
	}

	a.broadcastConfigUpdate(cfg)
	if a.notificationManager != nil {
		a.notificationManager.OnAutoControlChanged(enabled)
	}
	return nil
}

func (a *CoreApp) applyCurrentGearSetting() {
	fanData := a.deviceManager.GetCurrentFanData()
	if fanData == nil {
		return
	}
	cfg := a.configManager.Get()
	success := a.deviceManager.SetManualGear(fanData.SetGear, cfg.ManualLevel)
	if !success {
		a.logError("按当前设备挡位恢复手动挡失败", "gear", fanData.SetGear, "level", cfg.ManualLevel)
	}

	if success && a.isConnected {
		a.safeGo("restoreCurrentRGB-applyGear", func() {
			time.Sleep(200 * time.Millisecond)
			a.restoreCurrentRGBSilently()
		})
	}
}

func (a *CoreApp) SetManualGear(gear, level string) bool {
	cfg := a.configManager.Get()
	cfg.ManualGear = gear
	cfg.ManualLevel = level
	if err := a.configManager.Update(cfg); err != nil {
		a.logError("保存手动挡位配置失败", "gear", gear, "level", level, "error", err)
	}

	success := a.deviceManager.SetManualGear(gear, level)
	if !success {
		a.logError("手动挡位下发失败", "gear", gear, "level", level, "connected", a.isConnected)
	}

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
	isConnected := a.isConnected
	err := a.configManager.Update(cfg)
	a.mutex.Unlock()

	if enabled && isConnected {
		a.safeGo("setCustomFanSpeed", func() {
			if !a.deviceManager.SetCustomFanSpeed(rpm) {
				a.logError("设置自定义转速失败", "rpm", rpm, "stage", "user_apply")
			}
		})
	}

	a.broadcastConfigUpdate(cfg)

	if isConnected {
		a.safeGo("restoreCurrentRGB-customSpeed", func() {
			time.Sleep(200 * time.Millisecond)
			a.restoreCurrentRGB()
		})
	}

	return err
}

func (a *CoreApp) applyDeviceFlag(action string, deviceCall func() bool, updateCfg func(*types.AppConfig), args ...any) bool {
	if !deviceCall() {
		logArgs := append([]any{"action", action}, args...)
		a.logError("设备设置下发失败", logArgs...)
		return false
	}
	cfg := a.configManager.Get()
	updateCfg(&cfg)
	if err := a.configManager.Update(cfg); err != nil {
		logArgs := append(append([]any{"action", action}, args...), "error", err)
		a.logError("保存设备设置配置失败", logArgs...)
	}
	a.broadcastConfigUpdate(cfg)
	return true
}

func (a *CoreApp) SetGearLight(enabled bool) bool {
	return a.applyDeviceFlag(
		"gear_light",
		func() bool { return a.deviceManager.SetGearLight(enabled) },
		func(cfg *types.AppConfig) { cfg.GearLight = enabled },
		"enabled", enabled,
	)
}

func (a *CoreApp) SetPowerOnStart(enabled bool) bool {
	return a.applyDeviceFlag(
		"power_on_start",
		func() bool { return a.deviceManager.SetPowerOnStart(enabled) },
		func(cfg *types.AppConfig) { cfg.PowerOnStart = enabled },
		"enabled", enabled,
	)
}

func (a *CoreApp) SetSmartStartStop(mode string) bool {
	return a.applyDeviceFlag(
		"smart_start_stop",
		func() bool { return a.deviceManager.SetSmartStartStop(mode) },
		func(cfg *types.AppConfig) { cfg.SmartStartStop = mode },
		"mode", mode,
	)
}

func (a *CoreApp) SetBrightness(percentage int) bool {
	return a.applyDeviceFlag(
		"brightness",
		func() bool { return a.deviceManager.SetBrightness(percentage) },
		func(cfg *types.AppConfig) { cfg.Brightness = percentage },
		"brightness", percentage,
	)
}

func (a *CoreApp) SetRGBMode(params ipc.SetRGBModeParams) bool {
	return a.applyRGBMode(params, true)
}

func (a *CoreApp) applyRGBMode(params ipc.SetRGBModeParams, notify bool) bool {
	if !a.isConnected {
		a.logError("RGB 下发失败：设备未连接", "mode", params.Mode, "colors", len(params.Colors), "speed", params.Speed, "brightness", params.Brightness)
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

		level := smartRGBLevelForTemp(curTemp)
		success = rgbCtrl.SetSmartTempLevel(level)
		if !success {
			a.logError("智能 RGB 下发失败", "level", level, "temp", curTemp)
		}
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
		a.logError("RGB 下发失败：模式不支持", "mode", params.Mode, "colors", len(params.Colors), "speed", params.Speed, "brightness", params.Brightness)
		return false
	}
	if !success {
		a.logError("RGB 下发失败", "mode", params.Mode, "colors", len(params.Colors), "speed", params.Speed, "brightness", params.Brightness, "notify", notify)
	}

	if success {
		cfg := a.configManager.Get()
		cfg.HotkeyConflicts = append([]types.HotkeyConflict{}, a.systemHotkeyConflicts...)
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
		if err := a.configManager.Update(cfg); err != nil {
			a.logError("保存 RGB 配置失败", "mode", params.Mode, "error", err)
		}
		a.broadcastConfigUpdate(cfg)
		if notify && a.notificationManager != nil {
			a.notificationManager.OnRGBModeChanged(describeRGBMode(params.Mode))
		}
	}
	return success
}

func (a *CoreApp) setSystemHotkeyConflicts(conflicts []types.HotkeyConflict) {
	a.mutex.Lock()
	if len(conflicts) == 0 {
		a.systemHotkeyConflicts = nil
	} else {
		a.systemHotkeyConflicts = append([]types.HotkeyConflict{}, conflicts...)
	}
	cfg := a.configManager.Get()
	cfg.HotkeyConflicts = append([]types.HotkeyConflict{}, a.systemHotkeyConflicts...)
	a.mutex.Unlock()
	a.broadcastConfigUpdate(cfg)
}

func (a *CoreApp) CycleRGBMode() bool {
	cfg := a.configManager.Get()
	modes := []string{"smart", "rotation", "breathing", "static_single", "static_multi", "flowing", "off"}
	current := "smart"
	if cfg.RGBConfig != nil && cfg.RGBConfig.Mode != "" {
		current = cfg.RGBConfig.Mode
	}
	index := 0
	for i, mode := range modes {
		if mode == current {
			index = i
			break
		}
	}
	nextMode := modes[(index+1)%len(modes)]
	params := rgbParamsFromConfig(cfg)
	params.Mode = nextMode
	return a.SetRGBMode(params)
}

func describeRGBMode(mode string) string {
	switch mode {
	case "smart":
		return "智能"
	case "rotation":
		return "旋转"
	case "breathing":
		return "呼吸"
	case "static_single":
		return "单色常亮"
	case "static_multi":
		return "多色常亮"
	case "flowing":
		return "流光"
	case "off":
		return "关闭"
	default:
		return mode
	}
}

func (a *CoreApp) GetDebugInfo() map[string]any {
	a.mutex.RLock()
	debugMode := a.debugMode
	isConnected := a.isConnected
	monitoringTemp := a.monitoringTemp
	a.mutex.RUnlock()

	return map[string]any{
		"debugMode":      debugMode,
		"isConnected":    isConnected,
		"monitoringTemp": monitoringTemp,
		"hasGUIClients":  a.ipcServer != nil && a.ipcServer.HasClients(),
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
	err := a.configManager.Update(cfg)
	a.mutex.Unlock()
	if err != nil {
		return err
	}
	a.broadcastConfigUpdate(cfg)
	return nil
}

func (a *CoreApp) triggerHotkeyAction(action string) {
	switch action {
	case "show-main-window":
		a.logInfo("show-main-window 已改为由 Monitor 本地处理")
	case "toggle-auto-control":
		a.toggleAutoControlHotkey()
	case "toggle-process-switch":
		a.toggleProcessSwitchHotkey()
	case "cycle-rgb-mode":
		a.CycleRGBMode()
	case "device-toggle-offset":
		a.toggleOffsetHotkey()
	case "device-gear-quiet":
		a.triggerManualGearHotkey("静音")
	case "device-gear-standard":
		a.triggerManualGearHotkey("标准")
	case "device-gear-strong":
		a.triggerManualGearHotkey("强劲")
	case "device-gear-overclock":
		a.triggerManualGearHotkey("超频")
	case "device-custom-speed-toggle":
		a.toggleCustomSpeedHotkey()
	case "device-custom-speed-apply":
		a.applyCustomSpeedHotkey()
	default:
		if a.ipcServer != nil {
			a.ipcServer.BroadcastEventToRole(ipc.RoleGUI, ipc.EventHotkeyAction, ipc.TriggerHotkeyActionParams{Action: action})
		}
	}
}

func (a *CoreApp) toggleAutoControlHotkey() {
	cfg := a.configManager.Get()
	next := !cfg.AutoControl
	if err := a.SetAutoControl(next); err != nil {
		a.logError("快捷键切换智能变频失败", "current", cfg.AutoControl, "next", next, "error", err)
		a.emitNotification(notification.Request{Title: "智能变频切换失败", Message: err.Error()})
	}
}

func (a *CoreApp) toggleProcessSwitchHotkey() {
	cfg := a.configManager.Get()
	next := !cfg.ProcessSwitchEnabled
	if err := a.SetProcessSwitchEnabled(next); err != nil {
		a.logError("快捷键切换进程联动失败", "current", cfg.ProcessSwitchEnabled, "next", next, "error", err)
		a.emitNotification(notification.Request{Title: "进程联动切换失败", Message: err.Error()})
	}
}

func (a *CoreApp) toggleOffsetHotkey() {
	cfg := a.configManager.Get()
	if !cfg.AutoControl {
		a.emitNotification(notification.Request{Title: "自动曲线偏移不可用", Message: "请先开启智能变频后再切换自动曲线偏移"})
		return
	}
	cfg.FanCurveOffsetEnabled = !cfg.FanCurveOffsetEnabled
	if cfg.FanCurveOffsetEnabled && cfg.TempSampleCount < 3 {
		cfg.TempSampleCount = 3
	}
	if err := a.UpdateConfig(cfg); err == nil {
		a.emitNotification(notification.Request{Title: "自动曲线偏移已切换", Message: boolMessage(cfg.FanCurveOffsetEnabled, "自动曲线偏移已开启", "自动曲线偏移已关闭")})
	}
}

func (a *CoreApp) toggleCustomSpeedHotkey() {
	cfg := a.configManager.Get()
	next := !cfg.CustomSpeedEnabled
	if err := a.SetCustomSpeed(next, cfg.CustomSpeedRPM); err == nil {
		a.emitNotification(notification.Request{Title: "自定义转速已切换", Message: boolMessage(next, fmt.Sprintf("自定义转速已开启：%d RPM", cfg.CustomSpeedRPM), "自定义转速已关闭")})
	}
}

func (a *CoreApp) applyCustomSpeedHotkey() {
	cfg := a.configManager.Get()
	if !cfg.CustomSpeedEnabled {
		a.emitNotification(notification.Request{Title: "自定义转速不可应用", Message: "请先开启自定义转速后再应用"})
		return
	}
	if err := a.SetCustomSpeed(true, cfg.CustomSpeedRPM); err == nil {
		a.emitNotification(notification.Request{Title: "自定义转速已应用", Message: fmt.Sprintf("当前已应用 %d RPM", cfg.CustomSpeedRPM)})
	}
}

func boolMessage(enabled bool, onText string, offText string) string {
	if enabled {
		return onText
	}
	return offText
}

func (a *CoreApp) triggerManualGearHotkey(gear string) {
	cfg := a.configManager.Get()
	if cfg.AutoControl || cfg.CustomSpeedEnabled {
		a.emitNotification(notification.Request{Title: "挡位切换不可用", Message: "当前模式下手动挡位被禁用，请先关闭智能变频或自定义转速"})
		return
	}
	if !a.SetManualGear(gear, cfg.ManualLevel) {
		a.emitNotification(notification.Request{Title: "挡位切换失败", Message: fmt.Sprintf("切换到%s挡失败", gear)})
		return
	}
	a.emitNotification(notification.Request{Title: "挡位已切换", Message: fmt.Sprintf("当前已切换到%s挡 - %s", gear, cfg.ManualLevel)})
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

	cfg := a.configManager.Get()

	if isConnected && cfg.AutoControl {
		if err := a.deviceManager.EnterAutoMode(); err != nil {
			a.logError("进入自动模式失败", "error", err)
		}
		time.Sleep(100 * time.Millisecond)
		cfg = a.configManager.Get()
	}

	intervalSec := cfg.TempUpdateRate
	if intervalSec < 1 {
		intervalSec = 1
	}
	// 偏移开启时，采样间隔至少3秒，与偏移控制器冷却期对齐
	if cfg.FanCurveOffsetEnabled && intervalSec < 3 {
		intervalSec = 3
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

		// 温度读取失败重试计数器
		tempReadFailCount := 0
		const maxTempReadFailures = 3

		for {
			select {
			case <-a.stopMonitoring:
				return

			case <-ticker.C:
				cfg := a.configManager.Get()
				hasGUI := a.ipcServer != nil && a.ipcServer.HasClients()

				if !cfg.AutoControl && !hasGUI {
					continue
				}

				temp := a.tempReader.Read()
				if temp.MaxTemp <= 0 {
					tempReadFailCount++
					if tempReadFailCount <= maxTempReadFailures {
						a.logDebug("温度读取失败", "attempt", tempReadFailCount, "max_attempts", maxTempReadFailures, "reason", temp.BridgeMsg)
						continue
					} else {
						a.logInfo("温度读取连续失败，继续执行但智能变频可能不生效", "attempt", maxTempReadFailures, "reason", temp.BridgeMsg)
					}
				} else {
					// 温度读取成功，重置失败计数器
					if tempReadFailCount > 0 {
						a.logInfo("温度读取恢复成功")
						tempReadFailCount = 0
					}
				}

				a.mutex.Lock()
				a.currentTemp = temp
				processSwitchActive := a.activeProcessProfilePath != ""
				a.mutex.Unlock()

				// 将自动偏移量附加到温度数据中，供 GUI 展示
				if cfg.FanCurveOffsetEnabled && !processSwitchActive {
					displayCurve := a.visibleFanCurve(cfg.FanCurve)
					temp.AutoOffset = temperature.CalculateOffset(temp.MaxTemp, displayCurve)
					temp.EngineState = a.fanOffsetCtrl.GetCurrentZoneState(temp.MaxTemp, displayCurve)
				}

				if a.ipcServer != nil {
					go func(t types.TemperatureData) {
						defer func() { _ = recover() }()
						a.ipcServer.BroadcastEvent(ipc.EventTemperatureUpdate, t)
					}(temp)
				}

				// 分离式 RGB 智能温控判定
				if cfg.RGBConfig.Mode == "smart" && temp.MaxTemp > 0 {
					var level byte
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
					// 偏移开启时，平滑采样至少3次，确保趋势数据不受噪声干扰
					if cfg.FanCurveOffsetEnabled && !processSwitchActive && newSampleCount < 3 {
						newSampleCount = 3
					}
					if newSampleCount != sampleCount {
						sampleCount = newSampleCount
						tempSamples = make([]int, 0, sampleCount)
					}
					// 动态响应采样间隔配置变更
					newIntervalSec := cfg.TempUpdateRate
					if newIntervalSec < 1 {
						newIntervalSec = 1
					}
					// 偏移开启时，采样间隔至少3秒
					if cfg.FanCurveOffsetEnabled && !processSwitchActive && newIntervalSec < 3 {
						newIntervalSec = 3
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

					var effectiveCurve []types.FanCurvePoint
					var latestFanData *types.FanData
					if cfg.FanCurveOffsetEnabled && !processSwitchActive {
						effectiveCurve = a.visibleFanCurve(cfg.FanCurve)
					} else {
						effectiveCurve = cfg.FanCurve
					}
					if cfg.FanCurveOffsetEnabled && !processSwitchActive {
						// 计算设备最大RPM (根据挡位)
						deviceMaxRPM := 4000
						if fd := a.deviceManager.GetCurrentFanData(); fd != nil {
							latestFanData = fd
							switch fd.MaxGear {
							case "静音":
								deviceMaxRPM = 1900
							case "标准":
								deviceMaxRPM = 2760
							case "强劲":
								deviceMaxRPM = 3300
							case "超频":
								deviceMaxRPM = 4000
							}
						}
						changed := a.fanOffsetCtrl.Update(avgTemp, effectiveCurve, 500, deviceMaxRPM)
						if changed {
							if a.setRuntimeFanCurve(effectiveCurve) && a.ipcServer != nil {
								go func(c types.AppConfig) {
									defer func() { _ = recover() }()
									a.broadcastConfigUpdate(c)
								}(cfg)
							}
						}
					}
					avgBasedTargetRPM := temperature.CalculateTargetRPM(avgTemp, effectiveCurve)
					instantTargetRPM := temperature.CalculateTargetRPM(temp.MaxTemp, effectiveCurve)
					if cfg.FanCurveOffsetEnabled && !processSwitchActive {
						avgBasedTargetRPM = temperature.ApplyOffset(avgBasedTargetRPM, temperature.CalculateOffset(avgTemp, effectiveCurve))
						instantTargetRPM = temperature.ApplyOffset(instantTargetRPM, temperature.CalculateOffset(temp.MaxTemp, effectiveCurve))
					}
					thermalTargetRPM := avgBasedTargetRPM
					if instantTargetRPM > thermalTargetRPM {
						thermalTargetRPM = instantTargetRPM
					}
					if latestFanData == nil {
						latestFanData = a.deviceManager.GetCurrentFanData()
					}
					now := time.Now()
					plan := a.planFanCommandTarget(now, thermalTargetRPM, temp.MaxTemp, latestFanData)
					if latestFanData != nil {
						rpmError := plan.CommandTargetRPM - int(latestFanData.CurrentRPM)
						targetGap := int(latestFanData.TargetRPM) - int(latestFanData.CurrentRPM)
						a.fanOffsetCtrl.ObserveFanResponse(avgTemp, plan.CommandTargetRPM, int(latestFanData.CurrentRPM), int(latestFanData.TargetRPM))
						if plan.ShouldSend || plan.StateChanged {
							a.logDebug("智能变频监控计算结果", "thermal_target_rpm", plan.ThermalTargetRPM, "target_rpm", plan.CommandTargetRPM, "shape_reason", plan.ShapeReason, "avg_temp", avgTemp, "raw_temp", temp.MaxTemp, "curve_points", len(cfg.FanCurve), "current_rpm", latestFanData.CurrentRPM, "device_target_rpm", latestFanData.TargetRPM, "rpm_error", rpmError, "target_gap", targetGap, "mode", latestFanData.WorkMode, "gear", latestFanData.SetGear)
						}
					} else {
						a.fanOffsetCtrl.ObserveFanResponse(avgTemp, 0, 0, 0)
						if plan.ShouldSend || plan.StateChanged {
							a.logDebug("智能变频监控计算结果", "thermal_target_rpm", plan.ThermalTargetRPM, "target_rpm", plan.CommandTargetRPM, "shape_reason", plan.ShapeReason, "avg_temp", avgTemp, "raw_temp", temp.MaxTemp, "curve_points", len(cfg.FanCurve), "current_rpm", "unknown")
						}
					}
					if plan.ShouldSend && plan.CommandTargetRPM > 0 {
						blockedByModeRecovery := false
						if latestFanData != nil && latestFanData.WorkMode == "挡位工作模式" {
							if a.fanCommandShaper != nil && !a.fanCommandShaper.AllowModeRecovery(now) {
								blockedByModeRecovery = true
								if plan.StateChanged {
									a.logDebug("智能变频监控等待自动模式恢复节流", "thermal_target_rpm", plan.ThermalTargetRPM, "target_rpm", plan.CommandTargetRPM, "shape_reason", plan.ShapeReason, "avg_temp", avgTemp)
								}
							} else {
								a.logDebug("智能变频监控：设备进入手动模式，重新进入自动模式")
								if err := a.deviceManager.EnterAutoMode(); err == nil {
									time.Sleep(100 * time.Millisecond)
								} else {
									a.logError("智能变频监控恢复自动模式失败", "thermal_target_rpm", plan.ThermalTargetRPM, "target_rpm", plan.CommandTargetRPM, "avg_temp", avgTemp, "error", err)
								}
							}
						}
						if !blockedByModeRecovery {
							a.logDebug("智能变频监控下发新风扇转速", "thermal_target_rpm", plan.ThermalTargetRPM, "target_rpm", plan.CommandTargetRPM, "shape_reason", plan.ShapeReason)
							if !a.deviceManager.SetFanSpeed(plan.CommandTargetRPM) {
								if latestFanData != nil {
									a.logError("智能变频监控下发风扇转速失败", "thermal_target_rpm", plan.ThermalTargetRPM, "target_rpm", plan.CommandTargetRPM, "shape_reason", plan.ShapeReason, "avg_temp", avgTemp, "mode", latestFanData.WorkMode, "gear", latestFanData.SetGear)
								} else {
									a.logError("智能变频监控下发风扇转速失败", "thermal_target_rpm", plan.ThermalTargetRPM, "target_rpm", plan.CommandTargetRPM, "shape_reason", plan.ShapeReason, "avg_temp", avgTemp)
								}
							}
						}
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
	currentInterval := baseInterval

	for {
		select {
		case <-time.After(currentInterval):
			a.checkDeviceHealth(&currentInterval, baseInterval)
		case <-a.cleanupChan:
			return
		}
	}
}

func (a *CoreApp) checkDeviceHealth(currentInterval *time.Duration, baseInterval time.Duration) {
	a.mutex.RLock()
	paused := a.corePaused
	connected := a.isConnected
	a.mutex.RUnlock()
	if paused {
		*currentInterval = baseInterval
		return
	}

	if !connected {
		// scheduleReconnect 协程负责非用户断开后的重连（等待设备出现再连接），
		// Watchdog 不再重复尝试，避免双重连接竞争。
		*currentInterval = baseInterval
		return
	} else {
		// 连接状态下，检查设备是否真的在线
		if !a.deviceManager.IsConnected() {
			a.logError("设备Watchdog: 检测到设备状态不一致，触发断开回调")
			a.onDeviceDisconnect()
			*currentInterval = baseInterval // 准备立即开始快速重连
		} else {
			// 设备在线，保持正常的心跳频率
			*currentInterval = baseInterval
		}
	}
}

func (a *CoreApp) cleanup() {
	select {
	case a.cleanupChan <- true:
	default:
	}
	if a.logger != nil {
		a.logger.Close()
	}
}

func (a *CoreApp) logInfo(msg any, args ...any) {
	if a.logger != nil {
		a.logger.Info(msg, args...)
	}
}

func (a *CoreApp) logError(msg any, args ...any) {
	if a.logger != nil {
		a.logger.Error(msg, args...)
	}
}

func (a *CoreApp) logDebug(msg any, args ...any) {
	if a.logger != nil {
		a.logger.Debug(msg, args...)
	}
}

// smartRGBLevelForTemp 根据当前温度映射智能灯光档位
func smartRGBLevelForTemp(temp int) byte {
	level := byte(1)
	if temp > 0 {
		if temp < 60 {
			level = 1
		} else if temp < 85 {
			level = 2
		} else if temp < 90 {
			level = 3
		} else {
			level = 4
		}
	}
	return level
}

// rgbParamsFromConfig 从配置构造 RGB 参数
func rgbParamsFromConfig(cfg types.AppConfig) ipc.SetRGBModeParams {
	params := ipc.SetRGBModeParams{
		Mode:       cfg.RGBConfig.Mode,
		Colors:     make([]ipc.RGBColorParam, len(cfg.RGBConfig.Colors)),
		Speed:      cfg.RGBConfig.Speed,
		Brightness: cfg.RGBConfig.Brightness,
	}
	for i, c := range cfg.RGBConfig.Colors {
		params.Colors[i] = ipc.RGBColorParam{R: c.R, G: c.G, B: c.B}
	}
	return params
}

// restoreCurrentRGB 恢复当前配置的RGB设置
func (a *CoreApp) restoreCurrentRGB() {
	if !a.isConnected {
		a.logDebug("跳过恢复当前 RGB：设备未连接")
		return
	}
	params := rgbParamsFromConfig(a.configManager.Get())
	if !a.SetRGBMode(params) {
		a.logError("恢复当前 RGB 失败", "mode", params.Mode, "colors", len(params.Colors), "speed", params.Speed, "brightness", params.Brightness)
	}
}

// restoreCurrentRGBSilently 静默恢复当前配置的RGB设置
func (a *CoreApp) restoreCurrentRGBSilently() {
	if !a.isConnected {
		a.logDebug("跳过静默恢复当前 RGB：设备未连接")
		return
	}
	params := rgbParamsFromConfig(a.configManager.Get())
	a.logDebug("静默恢复当前 RGB", "mode", params.Mode, "colors", len(params.Colors), "speed", params.Speed, "brightness", params.Brightness)
	if !a.applyRGBMode(params, false) {
		a.logError("静默恢复当前 RGB 失败", "mode", params.Mode, "colors", len(params.Colors), "speed", params.Speed, "brightness", params.Brightness)
	}
}

// restoreSmartRGBForAutoControl 在开启智能变频时，仅对智能灯光模式静默补应用一次
func (a *CoreApp) restoreSmartRGBForAutoControl() {
	if !a.isConnected {
		a.logDebug("跳过恢复智能 RGB：设备未连接")
		return
	}

	cfg := a.configManager.Get()
	if cfg.RGBConfig == nil {
		a.logDebug("跳过恢复智能 RGB：RGB 配置为空")
		return
	}
	if cfg.RGBConfig.Mode != "smart" {
		a.logDebug("跳过恢复智能 RGB：当前模式不是智能模式", "mode", cfg.RGBConfig.Mode)
		return
	}

	a.mutex.Lock()
	a.lastSmartModeLevel = 0
	curTemp := a.currentTemp.MaxTemp
	a.mutex.Unlock()

	level := smartRGBLevelForTemp(curTemp)
	if !a.deviceManager.RGB().SetSmartTempLevel(level) {
		a.logError("恢复智能 RGB 失败", "mode", cfg.RGBConfig.Mode, "level", level, "temp", curTemp)
		return
	}
	a.mutex.Lock()
	a.lastSmartModeLevel = level
	a.mutex.Unlock()
}

func (a *CoreApp) RestartService() {
	a.mutex.Lock()
	wasPaused := a.corePaused
	a.corePaused = false
	a.mutex.Unlock()
	if !wasPaused {
		a.logInfo("核心恢复请求已忽略：当前未暂停")
		return
	}
	if a.ConnectDevice() {
		a.logInfo("核心已恢复并重新连接设备")
		return
	}
	gen := atomic.AddInt32(&a.reconnectGen, 1)
	go a.scheduleReconnect(gen)
	a.logInfo("核心已恢复，设备连接延后重试")
}

func (a *CoreApp) StopService() {
	a.mutex.Lock()
	if a.corePaused {
		a.mutex.Unlock()
		a.logInfo("核心暂停请求已忽略：当前已处于暂停状态")
		return
	}
	a.corePaused = true
	if a.monitoringTemp {
		select {
		case a.stopMonitoring <- true:
		default:
		}
		a.monitoringTemp = false
	}
	a.mutex.Unlock()
	atomic.AddInt32(&a.reconnectGen, 1)
	a.DisconnectDevice()
	a.logInfo("核心已进入暂停态，当前仅保留 IPC 与恢复入口")
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
