package main

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TIANLI0/BS2PRO-Controller/internal/autostart"
	"github.com/TIANLI0/BS2PRO-Controller/internal/config"
	"github.com/TIANLI0/BS2PRO-Controller/internal/ipc"
	"github.com/TIANLI0/BS2PRO-Controller/internal/logger"
	"github.com/TIANLI0/BS2PRO-Controller/internal/notification"
	"github.com/TIANLI0/BS2PRO-Controller/internal/platformutil"
	"github.com/TIANLI0/BS2PRO-Controller/internal/types"
	"github.com/TIANLI0/BS2PRO-Controller/internal/version"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct - GUI 应用程序结构
type App struct {
	ctx            context.Context
	ipcClient      *ipc.Client
	mutex          sync.RWMutex
	monitorStateMu sync.Mutex
	iconData       []byte
	windowVisible  bool
	monitorDesired bool

	// 缓存的状态 (托盘和前端随时读取)
	isConnected                 bool
	coreRunning                 bool
	monitorManaged              bool
	currentTemp                 types.TemperatureData
	currentFan                  *types.FanData
	autoControlState            bool
	registerGUIRoleRunning      atomic.Bool
	registeredGUIRoleGeneration atomic.Int64
	showWindowInFlight          atomic.Bool

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

var guiLogger *logger.CustomLogger

func init() {
	guiLogger = initGUILogger()
}

func initGUILogger() *logger.CustomLogger {
	baseDirs := []string{
		filepath.Dir(config.GetLogDir()),
		os.TempDir(),
		".",
	}
	for _, baseDir := range baseDirs {
		customLogger, err := logger.NewCustomLogger(false, baseDir, "gui")
		if err != nil {
			continue
		}
		customLogger.CleanOldLogs()
		return customLogger
	}
	return nil
}

// GUI 日志包装函数，保持与其他包调用层数一致
func logInfo(msg any, args ...any) {
	if guiLogger != nil {
		guiLogger.Info(msg, args...)
	}
}

func logError(msg any, args ...any) {
	if guiLogger != nil {
		guiLogger.Error(msg, args...)
	}
}

func logWarn(msg any, args ...any) {
	if guiLogger != nil {
		guiLogger.Warn(msg, args...)
	}
}

func logDebug(msg any, args ...any) {
	if guiLogger != nil {
		guiLogger.Debug(msg, args...)
	}
}

func closeWithLog(name string, closer io.Closer) {
	if closer == nil {
		return
	}
	if err := closer.Close(); err != nil {
		logWarn("关闭资源失败", "resource", name, "error", err)
	}
}

// NewApp 创建 GUI 应用实例
func NewApp(icon []byte) *App {
	return &App{
		ipcClient:     ipc.NewClient(nil),
		coreRunning:   true,
		currentTemp:   types.TemperatureData{BridgeOk: true},
		iconData:      icon,
		windowVisible: true,
	}
}

// startup 应用启动时调用
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	logInfo("GUI 启动", "version", version.Get())

	isAutoStart := false
	for _, arg := range os.Args[1:] {
		if arg == "--autostart" || arg == "/autostart" || arg == "-autostart" {
			isAutoStart = true
			break
		}
	}
	a.mutex.Lock()
	a.windowVisible = !isAutoStart
	a.mutex.Unlock()

	// 初始化自启动管理器
	a.autostartManager = autostart.NewManager(guiLogger, config.GetInstallDir())
	a.syncMonitorAgentState()

	// 提前注册事件处理器，确保 Watchdog 重连时也能触发前端通知
	a.ipcClient.SetEventHandler(a.handleCoreEvent)

	// 连接到后台核心服务
	if err := a.ipcClient.Connect(); err != nil {
		logError("连接核心服务失败", "error", err)
		runtime.EventsEmit(ctx, "core-service-error", "无法连接到核心服务，请检查服务是否运行")

		go func() {
			defaultCfg := types.GetDefaultConfig(false)
			defaultCfg = normalizeMonitorConfig(defaultCfg)
			defaultCfg.WindowsAutoStart = a.autostartManager.CheckWindowsAutoStart()
			runtime.EventsEmit(ctx, "config-update", defaultCfg)
		}()
	} else {
		logInfo("已成功连接到核心服务 IPC 管道")
		a.scheduleRegisterGUIClientRole("startup-connect")

		// 启动时主动拉取一次配置，同步状态
		cfg := a.GetConfig()
		status := a.GetDeviceStatus()
		logDebug("启动阶段状态快照",
			"config_tray", cfg.TrayEnabled,
			"config_notifications", cfg.NotificationsEnabled,
			"config_hotkeys", hotkeysRequireMonitor(cfg),
			"config_monitor_autostart", cfg.MonitorAutoStart,
			"status_connected", status["connected"],
			"status_model", status["model"],
			"status_product_id", status["productId"])
		cfg.WindowsAutoStart = a.autostartManager.CheckWindowsAutoStart()
		a.syncMonitorAutoStartFromConfig("startup-connect")

		a.mutex.Lock()
		a.autoControlState = cfg.AutoControl
		a.coreRunning = true
		if connected, ok := status["connected"].(bool); ok {
			a.isConnected = connected
		}
		if paused, ok := status["paused"].(bool); ok {
			a.coreRunning = !paused
		} else {
			a.coreRunning = true
		}
		a.mutex.Unlock()
		go func() {
			runtime.EventsEmit(ctx, "config-update", cfg)
			if a.isConnected {
				runtime.EventsEmit(ctx, "device-connected", status["currentData"])
			}
		}()
	}

	// 启动连接健康检查
	go a.startConnectionHealthCheck()

	if !isAutoStart {
		go func() {
			time.Sleep(300 * time.Millisecond)
			logInfo("首次非静默启动，主动显示主窗口")
			a.ShowWindow()
		}()
	} else {
		logInfo("当前为静默启动，跳过主动显示主窗口")
	}

	logInfo("GUI 启动完成")
}

func (a *App) scheduleRegisterGUIClientRole(reason string) {
	if !a.registerGUIRoleRunning.CompareAndSwap(false, true) {
		if guiLogger != nil {
			guiLogger.Debug("GUI 客户端角色注册已在进行",
				"reason", reason)
		}
		return
	}
	go func() {
		defer a.registerGUIRoleRunning.Store(false)
		for attempt := 1; attempt <= 6; attempt++ {
			if a.ipcClient == nil || !a.ipcClient.IsConnected() {
				if guiLogger != nil {
					guiLogger.Warn("注册 GUI 客户端角色等待连接",
						"reason", reason,
						"attempt", attempt)
				}
				time.Sleep(300 * time.Millisecond)
				continue
			}
			generation := a.ipcClient.ConnectionGeneration()
			if generation == 0 {
				if guiLogger != nil {
					guiLogger.Warn("注册 GUI 客户端角色等待连接代稳定",
						"reason", reason,
						"attempt", attempt)
				}
				time.Sleep(300 * time.Millisecond)
				continue
			}
			if a.registeredGUIRoleGeneration.Load() == generation {
				if guiLogger != nil {
					guiLogger.Debug("GUI 客户端角色已完成当前连接代注册",
						"reason", reason,
						"generation", generation)
				}
				return
			}
			resp, err := a.ipcClient.SendRequest(ipc.ReqRegisterClient, ipc.RegisterClientParams{Role: ipc.RoleGUI})
			if err == nil && resp != nil && resp.Success {
				a.registeredGUIRoleGeneration.Store(generation)
				if guiLogger != nil {
					guiLogger.Info("已注册 GUI 客户端角色",
						"reason", reason,
						"attempt", attempt,
						"generation", generation)
				}
				return
			}
			if guiLogger != nil {
				guiLogger.Warn("注册 GUI 客户端角色失败",
					"reason", reason,
					"attempt", attempt,
					"generation", generation,
					"resp_nil", resp == nil,
					"resp_success", resp != nil && resp.Success,
					"error", err)
			}
			time.Sleep(300 * time.Millisecond)
		}
		if guiLogger != nil {
			guiLogger.Error("注册 GUI 客户端角色最终失败",
				"reason", reason)
		}
	}()
}

func (a *App) ensureMonitorAgentReady() {
	if err := a.startProcessSwitchMonitor(); err != nil {
		logError("启动 Monitor 失败", "error", err)
	} else {
		logInfo("已确保 Monitor 运行")
	}
}

func (a *App) shouldMonitorBeRunning() bool {
	return a.shouldEnsureMonitorAgent()
}

func normalizeMonitorConfig(cfg AppConfig) AppConfig {
	cfg.Repair()
	return cfg
}

func (a *App) getMonitorDecisionConfig() (AppConfig, string) {
	if a.ipcClient != nil && a.ipcClient.IsConnected() {
		return normalizeMonitorConfig(a.GetConfig()), "ipc"
	}

	manager := config.NewManager(config.GetInstallDir(), guiLogger)
	cfg := normalizeMonitorConfig(manager.Load(false))
	cfg.WindowsAutoStart = a.CheckWindowsAutoStart()
	return cfg, "local"
}

func (a *App) shouldEnsureMonitorAgent() bool {
	cfg, source := a.getMonitorDecisionConfig()
	shouldRun := cfg.MonitorRequired()
	logInfo("Monitor 启动判定",
		"source", source,
		"tray", cfg.TrayEnabled,
		"notifications", cfg.NotificationsEnabled,
		"process_switch", cfg.ProcessSwitchEnabled,
		"hotkeys", hotkeysRequireMonitor(cfg),
		"monitor_autostart", cfg.MonitorAutoStart,
		"should_run", shouldRun)
	return shouldRun
}

func hotkeysRequireMonitor(cfg AppConfig) bool {
	return cfg.HotkeysRequireMonitor()
}

func (a *App) syncMonitorAutoStartFromConfig(reason string) {
	cfg, source := a.getMonitorDecisionConfig()
	desired := cfg.MonitorAutoStart
	actual := a.getAutostartManager().CheckMonitorAutoStart()
	if actual == desired {
		logDebug("Monitor 自启动已与配置一致", "reason", reason, "source", source, "enabled", desired)
		return
	}
	if err := a.getAutostartManager().SetMonitorAutoStart(desired); err != nil {
		logError("同步 Monitor 自启动失败", "reason", reason, "source", source, "desired", desired, "actual", actual, "error", err)
		return
	}
	logInfo("已同步 Monitor 自启动", "reason", reason, "source", source, "enabled", desired)
}

func (a *App) syncMonitorAgentState() {
	a.monitorStateMu.Lock()
	defer a.monitorStateMu.Unlock()

	shouldRun := a.shouldMonitorBeRunning()
	a.mutex.RLock()
	wasDesired := a.monitorDesired
	wasManaged := a.monitorManaged
	a.mutex.RUnlock()
	if shouldRun {
		if wasDesired && wasManaged && a.isMonitorProcessRunning() {
			return
		}
		a.mutex.Lock()
		a.monitorDesired = true
		a.mutex.Unlock()
		a.ensureMonitorAgentReady()
		a.mutex.Lock()
		a.monitorManaged = true
		a.mutex.Unlock()
		return
	}
	a.mutex.Lock()
	a.monitorDesired = false
	a.monitorManaged = false
	a.mutex.Unlock()
	processRunning := a.isMonitorProcessRunning()
	if !wasDesired && !wasManaged && !processRunning {
		return
	}
	if processRunning {
		if err := a.stopProcessSwitchMonitor(); err != nil {
			logError("停止 Monitor 失败", "error", err)
		} else {
			logInfo("已停止 Monitor 代理进程")
		}
	}
}

func (a *App) ensureMonitorAgentRunningIfNeeded() {
	a.monitorStateMu.Lock()
	defer a.monitorStateMu.Unlock()

	a.mutex.RLock()
	desired := a.monitorDesired
	a.mutex.RUnlock()
	running := a.isMonitorProcessRunning()
	if !desired {
		if !running {
			return
		}
		logWarn("检测到 Monitor 不应运行，执行关闭")
		if err := a.stopProcessSwitchMonitor(); err != nil {
			logError("关闭残留 Monitor 失败", "error", err)
		} else {
			logInfo("已关闭残留 Monitor 代理进程")
		}
		a.mutex.Lock()
		a.monitorManaged = false
		a.mutex.Unlock()
		return
	}
	if running {
		return
	}
	logWarn("检测到 Monitor 未运行，按期望状态重新拉起")
	a.ensureMonitorAgentReady()
	a.mutex.Lock()
	a.monitorManaged = true
	a.mutex.Unlock()
}

func (a *App) OnWindowClosing(ctx context.Context) bool {
	logInfo("窗口关闭，直接退出 GUI 进程")
	return false
}

// handleCoreEvent 处理核心服务推送的事件
func (a *App) handleCoreEvent(event ipc.Event) {
	defer func() {
		if r := recover(); r != nil {
			if guiLogger != nil {
				logError("处理核心事件发生异常", "event_type", event.Type, "error", r)
			}
		}
	}()
	if a.ctx == nil {
		return
	}

	logDebug("收到核心事件", "event_type", event.Type)

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
		if err := json.Unmarshal(event.Data, &deviceInfo); err != nil {
			logWarn("解析设备连接事件失败", "error", err)
		}
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
		if err := json.Unmarshal(event.Data, &errMsg); err != nil {
			logWarn("解析设备错误事件失败", "error", err)
			errMsg = string(event.Data)
		}
		runtime.EventsEmit(a.ctx, "device-error", errMsg)

	case ipc.EventServiceConnected:
		logInfo("核心服务连接事件 - UI 刷新")
		a.registeredGUIRoleGeneration.Store(0)
		a.scheduleRegisterGUIClientRole("service-connected")
		// 服务重连后延迟刷新
		go func() {
			time.Sleep(500 * time.Millisecond)
			cfg := a.GetConfig()
			status := a.GetDeviceStatus()
			logDebug("服务重连后状态快照",
				"config_tray", cfg.TrayEnabled,
				"config_notifications", cfg.NotificationsEnabled,
				"config_hotkeys", hotkeysRequireMonitor(cfg),
				"config_monitor_autostart", cfg.MonitorAutoStart,
				"status_connected", status["connected"],
				"status_model", status["model"],
				"status_product_id", status["productId"],
				"status_paused", status["paused"])

			a.mutex.Lock()
			if connected, ok := status["connected"].(bool); ok {
				a.isConnected = connected
			}
			if paused, ok := status["paused"].(bool); ok {
				a.coreRunning = !paused
			} else {
				a.coreRunning = true
			}
			a.autoControlState = cfg.AutoControl
			a.mutex.Unlock()

			if a.ctx != nil {
				// 发送恢复事件
				runtime.EventsEmit(a.ctx, "core-service-connected", nil)
				runtime.EventsEmit(a.ctx, "config-update", cfg)

				// 设备在线时同步通知前端
				if a.isConnected {
					runtime.EventsEmit(a.ctx, "device-connected", status["currentData"])
				}
			}
		}()

	case ipc.EventServiceDisconnected:
		logWarn("核心服务断开事件")
		a.mutex.Lock()
		a.isConnected = false
		a.coreRunning = false
		a.mutex.Unlock()

		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "core-service-error", "核心服务意外终止，正在尝试重连...")
			runtime.EventsEmit(a.ctx, "device-disconnected", nil)
		}

	case ipc.EventConfigUpdate:
		var cfg types.AppConfig
		if err := json.Unmarshal(event.Data, &cfg); err == nil {
			cfg = normalizeMonitorConfig(cfg)
			cfg.WindowsAutoStart = a.CheckWindowsAutoStart()
			a.syncMonitorAutoStartFromConfig("event-config-update")
			a.mutex.Lock()
			a.autoControlState = cfg.AutoControl
			a.mutex.Unlock()
			runtime.EventsEmit(a.ctx, "config-update", cfg)
		}

	case ipc.EventHotkeyAction:
		var params ipc.TriggerHotkeyActionParams
		if err := json.Unmarshal(event.Data, &params); err == nil && params.Action != "" {
			runtime.EventsEmit(a.ctx, ipc.EventHotkeyAction, params.Action)
		}
	}
}

func (a *App) ToggleWindowVisibility() {
	a.mutex.RLock()
	visible := a.windowVisible
	a.mutex.RUnlock()
	if visible {
		a.HideWindow()
		return
	}
	a.ShowWindow()
}

// sendRequest 发送请求到核心服务
func (a *App) sendRequest(reqType ipc.RequestType, data any) (*ipc.Response, error) {
	return a.ipcClient.SendRequest(reqType, data)
}

func (a *App) GetAppVersion() string { return version.Get() }

func (a *App) SaveThemePreference(followSystem bool, mode string) error {
	return saveThemePreference(ThemePreference{FollowSystem: followSystem, Mode: mode})
}

func (a *App) ConnectDevice() bool {
	resp, err := a.sendRequest(ipc.ReqConnect, nil)
	if err != nil || resp == nil || !resp.Success {
		return false
	}
	var success bool
	if err := json.Unmarshal(resp.Data, &success); err != nil {
		logWarn("解析连接设备响应失败", "error", err)
		return false
	}
	return success
}

func (a *App) DisconnectDevice() {
	if _, err := a.sendRequest(ipc.ReqDisconnect, nil); err != nil {
		logWarn("断开设备请求失败", "error", err)
	}
}

func (a *App) GetDeviceStatus() map[string]any {
	resp, err := a.sendRequest(ipc.ReqGetDeviceStatus, nil)
	if err != nil || resp == nil || !resp.Success {
		logWarn("获取设备状态失败，回落为未连接",
			"resp_nil", resp == nil,
			"resp_success", resp != nil && resp.Success,
			"error", err)
		return map[string]any{"connected": false}
	}
	var status map[string]any
	if err := json.Unmarshal(resp.Data, &status); err != nil {
		logWarn("解析设备状态失败，回落为未连接", "error", err)
		return map[string]any{"connected": false}
	}
	logDebug("获取设备状态成功",
		"connected", status["connected"],
		"model", status["model"],
		"product_id", status["productId"],
		"paused", status["paused"])
	return status
}

func (a *App) GetConfig() AppConfig {
	resp, err := a.sendRequest(ipc.ReqGetConfig, nil)
	if err != nil || resp == nil || !resp.Success {
		logWarn("获取配置失败，回落默认配置",
			"resp_nil", resp == nil,
			"resp_success", resp != nil && resp.Success,
			"error", err)
		cfg := normalizeMonitorConfig(types.GetDefaultConfig(false))
		cfg.WindowsAutoStart = a.CheckWindowsAutoStart()
		logWarn("默认配置摘要",
			"tray", cfg.TrayEnabled,
			"notifications", cfg.NotificationsEnabled,
			"hotkeys_enabled", cfg.Hotkeys != nil && cfg.Hotkeys.Enabled,
			"global_hotkeys", func() int {
				if cfg.Hotkeys == nil {
					return 0
				}
				return len(cfg.Hotkeys.Global)
			}(),
			"inapp_hotkeys", func() int {
				if cfg.Hotkeys == nil {
					return 0
				}
				return len(cfg.Hotkeys.InApp)
			}())
		return cfg
	}
	var cfg AppConfig
	if err := json.Unmarshal(resp.Data, &cfg); err != nil {
		logWarn("解析配置失败，回落默认配置", "error", err)
		fallback := normalizeMonitorConfig(types.GetDefaultConfig(false))
		fallback.WindowsAutoStart = a.CheckWindowsAutoStart()
		return fallback
	}
	cfg = normalizeMonitorConfig(cfg)
	cfg.WindowsAutoStart = a.CheckWindowsAutoStart()
	logDebug("获取配置成功",
		"tray", cfg.TrayEnabled,
		"notifications", cfg.NotificationsEnabled,
		"hotkeys_enabled", cfg.Hotkeys != nil && cfg.Hotkeys.Enabled,
		"global_hotkeys", func() int {
			if cfg.Hotkeys == nil {
				return 0
			}
			return len(cfg.Hotkeys.Global)
		}(),
		"inapp_hotkeys", func() int {
			if cfg.Hotkeys == nil {
				return 0
			}
			return len(cfg.Hotkeys.InApp)
		}(),
		"monitor_autostart", cfg.MonitorAutoStart)
	return cfg
}

func (a *App) UpdateConfig(cfg AppConfig) error {
	cfg = normalizeMonitorConfig(cfg)
	oldCfg := a.GetConfig()
	resp, err := a.sendRequest(ipc.ReqUpdateConfig, cfg)
	if err != nil {
		return err
	}
	if resp == nil || !resp.Success {
		if resp != nil {
			return fmt.Errorf("%s", resp.Error)
		}
		return fmt.Errorf("服务返回为空")
	}
	if oldCfg.ProcessSwitchEnabled != cfg.ProcessSwitchEnabled {
		a.syncProcessSwitchMonitor(cfg.ProcessSwitchEnabled)
	}
	a.syncMonitorAutoStartFromConfig("update-config")
	if oldCfg.MonitorAutoStart != cfg.MonitorAutoStart {
		a.syncMonitorAgentState()
	}
	return nil
}

func (a *App) syncProcessSwitchMonitor(_ bool) {
	a.syncMonitorAgentState()
}

func (a *App) startProcessSwitchMonitor() error {
	monitorPath := filepath.Join(config.GetInstallDir(), "BS2PRO-Monitor.exe")
	if _, err := os.Stat(monitorPath); err != nil {
		return fmt.Errorf("monitor程序不存在: %s", monitorPath)
	}
	checkCmd := exec.Command("tasklist", "/FI", "IMAGENAME eq BS2PRO-Monitor.exe")
	platformutil.HideCommandWindow(checkCmd)
	out, err := checkCmd.Output()
	if err == nil && strings.Contains(strings.ToLower(string(out)), "bs2pro-monitor.exe") {
		logInfo("Monitor 已在运行，跳过重复拉起")
		return nil
	}
	cmd := exec.Command(monitorPath)
	cmd.Dir = filepath.Dir(monitorPath)
	platformutil.HideCommandWindow(cmd)
	return cmd.Start()
}

func (a *App) isMonitorProcessRunning() bool {
	checkCmd := exec.Command("tasklist", "/FI", "IMAGENAME eq BS2PRO-Monitor.exe")
	platformutil.HideCommandWindow(checkCmd)
	out, err := checkCmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "bs2pro-monitor.exe")
}

func (a *App) stopProcessSwitchMonitor() error {
	if !a.isMonitorProcessRunning() {
		return nil
	}
	cmd := exec.Command("taskkill", "/F", "/IM", "BS2PRO-Monitor.exe", "/T")
	platformutil.HideCommandWindow(cmd)
	return cmd.Run()
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
		return fmt.Errorf("服务返回为空")
	}
	return nil
}

func (a *App) ApplyOffsetToCurve() error {
	resp, err := a.sendRequest(ipc.ReqApplyOffsetToCurve, nil)
	if err != nil {
		return err
	}
	if resp == nil || !resp.Success {
		if resp != nil {
			return fmt.Errorf("%s", resp.Error)
		}
		return fmt.Errorf("服务返回为空")
	}
	return nil
}

func (a *App) GetFanCurve() []FanCurvePoint {
	resp, err := a.sendRequest(ipc.ReqGetFanCurve, nil)
	if err != nil || resp == nil || !resp.Success {
		return types.GetDefaultFanCurve()
	}
	var curve []FanCurvePoint
	if err := json.Unmarshal(resp.Data, &curve); err != nil {
		logWarn("解析风扇曲线失败", "error", err)
		return types.GetDefaultFanCurve()
	}
	return curve
}

type FanCurveProfileConfig struct {
	Name        string          `json:"name"`
	ProfilePath string          `json:"profilePath,omitempty"`
	FanCurve    []FanCurvePoint `json:"fanCurve"`
}

type fanCurveProfilesAppSettings struct {
	AutoControl bool   `json:"autoControl"`
	ManualGear  string `json:"manualGear"`
	ManualLevel string `json:"manualLevel"`
}

type fanCurveProfilesBundle struct {
	Version     string                      `json:"version"`
	ExportDate  string                      `json:"exportDate"`
	DeviceCurve []FanCurvePoint             `json:"deviceCurve"`
	Profiles    []FanCurveProfileConfig     `json:"profiles"`
	AppSettings fanCurveProfilesAppSettings `json:"appSettings"`
}

type fanCurveProfileFile struct {
	Name     string          `json:"name"`
	FanCurve []FanCurvePoint `json:"fanCurve"`
	SavedAt  string          `json:"savedAt,omitempty"`
}

func (a *App) GetFanCurveProfileConfigs() []FanCurveProfileConfig {
	cfg := a.GetConfig()
	dir := a.getFanCurveProfileDir(cfg)
	if dir == "" {
		return []FanCurveProfileConfig{}
	}

	files, err := filepath.Glob(filepath.Join(dir, "*-fan-config.json"))
	if err != nil || len(files) == 0 {
		return []FanCurveProfileConfig{}
	}

	result := make([]FanCurveProfileConfig, 0, len(files))
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}

		var payload fanCurveProfileFile
		if err := json.Unmarshal(data, &payload); err != nil {
			continue
		}
		if len(payload.FanCurve) == 0 {
			continue
		}
		if payload.Name == "" {
			base := filepath.Base(p)
			payload.Name = strings.TrimSuffix(base, "-fan-config.json")
		}
		result = append(result, FanCurveProfileConfig{
			Name:        payload.Name,
			ProfilePath: filepath.Base(p),
			FanCurve:    payload.FanCurve,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result
}

func (a *App) SaveFanCurveProfileConfigs(profiles []FanCurveProfileConfig) error {
	cfg := a.GetConfig()
	dir := a.getFanCurveProfileDir(cfg)
	if dir == "" {
		return fmt.Errorf("无法定位配置目录")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	oldFiles, _ := filepath.Glob(filepath.Join(dir, "*-fan-config.json"))
	for _, p := range oldFiles {
		_ = os.Remove(p)
	}

	used := map[string]int{}
	for i, profile := range profiles {
		name := strings.TrimSpace(profile.Name)
		if name == "" {
			name = "配置" + strconv.Itoa(i+1)
		}
		base := sanitizeProfileName(name)
		if base == "" {
			base = "config-" + strconv.Itoa(i+1)
		}
		seq := used[base]
		used[base] = seq + 1
		fileBase := base
		if seq > 0 {
			fileBase = fmt.Sprintf("%s-%d", base, seq+1)
		}

		outPath := filepath.Join(dir, fileBase+"-fan-config.json")
		payload := fanCurveProfileFile{
			Name:     name,
			FanCurve: profile.FanCurve,
			SavedAt:  time.Now().Format(time.RFC3339),
		}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(outPath, data, 0644); err != nil {
			return err
		}
	}

	return nil
}

func (a *App) getFanCurveProfileDir(cfg AppConfig) string {
	if cfg.ConfigPath != "" {
		return filepath.Dir(cfg.ConfigPath)
	}
	programData := os.Getenv("PROGRAMDATA")
	if programData != "" {
		return filepath.Join(programData, "BS2PRO-Controller")
	}
	return filepath.Join(config.GetInstallDir(), "config")
}

func sanitizeProfileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	b := strings.Builder{}
	for _, r := range name {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func (a *App) ExportFanCurveProfilesZip() error {
	if a.ctx == nil {
		return fmt.Errorf("窗口上下文不可用")
	}

	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "导出风扇配置包",
		DefaultFilename: fmt.Sprintf("bs2pro-fan-profiles-%s.zip", time.Now().Format("20060102")),
		Filters: []runtime.FileFilter{
			{DisplayName: "ZIP 文件", Pattern: "*.zip"},
		},
	})
	if err != nil {
		return err
	}
	if strings.TrimSpace(savePath) == "" {
		return nil
	}

	bundle := fanCurveProfilesBundle{
		Version:     "fan-profiles-zip-v1",
		ExportDate:  time.Now().Format(time.RFC3339),
		DeviceCurve: a.GetConfig().FanCurve,
		Profiles:    a.GetFanCurveProfileConfigs(),
		AppSettings: fanCurveProfilesAppSettings{
			AutoControl: a.GetConfig().AutoControl,
			ManualGear:  a.GetConfig().ManualGear,
			ManualLevel: a.GetConfig().ManualLevel,
		},
	}
	profileCount := len(bundle.Profiles)

	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}

	f, err := os.Create(savePath)
	if err != nil {
		return err
	}
	defer closeWithLog("风扇配置导出文件", f)

	zw := zip.NewWriter(f)
	w, err := zw.Create("fan-profiles.json")
	if err != nil {
		closeWithLog("风扇配置导出压缩流", zw)
		return err
	}
	if _, err := w.Write(data); err != nil {
		closeWithLog("风扇配置导出压缩流", zw)
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	_, _ = a.sendRequest(ipc.ReqNotifyExportCompleted, ipc.NotifyProfilesParams{ProfileCount: profileCount})
	return nil
}

func (a *App) ImportFanCurveProfilesZip() error {
	if a.ctx == nil {
		return fmt.Errorf("窗口上下文不可用")
	}

	openPath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "导入风扇配置包",
		Filters: []runtime.FileFilter{
			{DisplayName: "ZIP 文件", Pattern: "*.zip"},
		},
	})
	if err != nil {
		return err
	}
	if strings.TrimSpace(openPath) == "" {
		return nil
	}

	bundle, err := readFanProfilesBundleZip(openPath)
	if err != nil {
		return err
	}
	if err := validateFanProfilesBundle(bundle); err != nil {
		return err
	}

	if err := a.SaveFanCurveProfileConfigs(bundle.Profiles); err != nil {
		return err
	}

	cfg := a.GetConfig()
	if len(bundle.DeviceCurve) > 0 {
		cfg.FanCurve = bundle.DeviceCurve
	}
	cfg.AutoControl = bundle.AppSettings.AutoControl
	if strings.TrimSpace(bundle.AppSettings.ManualGear) != "" {
		cfg.ManualGear = bundle.AppSettings.ManualGear
	}
	if strings.TrimSpace(bundle.AppSettings.ManualLevel) != "" {
		cfg.ManualLevel = bundle.AppSettings.ManualLevel
	}

	if err := a.UpdateConfig(cfg); err != nil {
		return err
	}
	_, _ = a.sendRequest(ipc.ReqNotifyImportCompleted, ipc.NotifyProfilesParams{ProfileCount: len(bundle.Profiles)})
	return nil
}

func (a *App) ExportRecentLogsZip() error {
	if a.ctx == nil {
		return fmt.Errorf("窗口上下文不可用")
	}

	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "导出最近7天日志",
		DefaultFilename: fmt.Sprintf("bs2pro-logs-%s.zip", time.Now().Format("20060102")),
		Filters: []runtime.FileFilter{
			{DisplayName: "ZIP 文件", Pattern: "*.zip"},
		},
	})
	if err != nil {
		return err
	}
	if strings.TrimSpace(savePath) == "" {
		return nil
	}

	logDir := config.GetLogDir()
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return err
	}
	cutoff := time.Now().AddDate(0, 0, -7)

	f, err := os.Create(savePath)
	if err != nil {
		return err
	}
	defer closeWithLog("日志导出文件", f)

	zw := zip.NewWriter(f)
	defer func() {
		if zw != nil {
			closeWithLog("日志导出压缩流", zw)
		}
	}()

	count := 0
	diag, diagErr := a.buildFeedbackDiagnostics()
	if diagErr == nil {
		w, err := zw.Create("diagnostics/system-info.json")
		if err != nil {
			return err
		}
		if _, err := w.Write(diag); err != nil {
			return err
		}
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".log") {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().Before(cutoff) {
			continue
		}
		fullPath := filepath.Join(logDir, name)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		w, err := zw.Create(filepath.Join("logs", name))
		if err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
		count++
	}
	if count == 0 {
		return fmt.Errorf("最近7天没有可导出的日志")
	}
	if err := zw.Close(); err != nil {
		return err
	}
	zw = nil
	return nil
}

func (a *App) buildFeedbackDiagnostics() ([]byte, error) {
	cfg := a.GetConfig()
	debugInfo := map[string]any{}
	if info, err := a.sendRequest(ipc.ReqGetDebugInfo, nil); err == nil && info != nil && info.Success {
		_ = json.Unmarshal(info.Data, &debugInfo)
	}
	frontendEnv := map[string]any{}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "collect-frontend-diagnostics", nil)
	}
	webviewVersion := detectWebView2RuntimeVersion()
	hostname, _ := os.Hostname()
	payload := map[string]any{
		"exportedAt":          time.Now().Format(time.RFC3339),
		"appVersion":          version.Get(),
		"goos":                stdruntime.GOOS,
		"goarch":              stdruntime.GOARCH,
		"hostname":            hostname,
		"webview2Runtime":     webviewVersion,
		"config":              cfg,
		"coreDebugInfo":       debugInfo,
		"frontendDiagnostics": frontendEnv,
	}
	return json.MarshalIndent(payload, "", "  ")
}

func detectWebView2RuntimeVersion() string {
	queries := [][2]string{
		{`HKLM\SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`, "pv"},
		{`HKLM\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`, "pv"},
		{`HKCU\SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`, "pv"},
		{`HKLM\SOFTWARE\Microsoft\EdgeUpdate\ClientState\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`, "pv"},
		{`HKLM\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\ClientState\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`, "pv"},
		{`HKCU\SOFTWARE\Microsoft\EdgeUpdate\ClientState\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`, "pv"},
	}
	for _, q := range queries {
		if version := queryRegistryStringValue(q[0], q[1]); version != "" {
			return version
		}
	}
	return "unknown"
}

func queryRegistryStringValue(keyPath, valueName string) string {
	cmd := exec.Command("reg", "query", keyPath, "/v", valueName)
	platformutil.HideCommandWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "REG_SZ") {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				return parts[len(parts)-1]
			}
		}
	}
	return ""
}

func readFanProfilesBundleZip(path string) (*fanCurveProfilesBundle, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer closeWithLog("风扇配置导入压缩包", r)

	var target *zip.File
	for _, f := range r.File {
		if strings.EqualFold(filepath.Base(f.Name), "fan-profiles.json") {
			target = f
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("无效配置包：缺少 fan-profiles.json")
	}

	rc, err := target.Open()
	if err != nil {
		return nil, err
	}
	defer closeWithLog("风扇配置导入文件流", rc)

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	var bundle fanCurveProfilesBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return nil, fmt.Errorf("无效配置包：JSON 解析失败")
	}
	return &bundle, nil
}

func validateFanProfilesBundle(bundle *fanCurveProfilesBundle) error {
	if bundle == nil {
		return fmt.Errorf("无效配置包")
	}
	if bundle.Version != "fan-profiles-zip-v1" {
		return fmt.Errorf("无效配置包：版本不匹配")
	}
	if len(bundle.Profiles) == 0 {
		return fmt.Errorf("无效配置包：没有可导入的风扇配置")
	}
	for i, profile := range bundle.Profiles {
		if strings.TrimSpace(profile.Name) == "" {
			return fmt.Errorf("无效配置包：第 %d 个配置缺少名称", i+1)
		}
		if len(profile.FanCurve) == 0 {
			return fmt.Errorf("无效配置包：配置 %s 曲线为空", profile.Name)
		}
	}
	return nil
}

func (a *App) CheckProcessSwitchNow() bool {
	resp, err := a.sendRequest(ipc.ReqCheckProcessSwitchNow, nil)
	if err != nil || resp == nil || !resp.Success {
		return false
	}
	var success bool
	if err := json.Unmarshal(resp.Data, &success); err != nil {
		logWarn("解析进程切换立即检查结果失败", "error", err)
		return false
	}
	return success
}

func (a *App) ListRunningProcessNames() []string {
	cmd := exec.Command("tasklist", "/FO", "CSV", "/NH")
	platformutil.HideCommandWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return []string{}
	}

	r := csv.NewReader(strings.NewReader(string(out)))
	records, err := r.ReadAll()
	if err != nil {
		return []string{}
	}

	nameSet := map[string]struct{}{}
	for _, rec := range records {
		if len(rec) == 0 {
			continue
		}
		name := strings.TrimSpace(rec[0])
		if name == "" {
			continue
		}
		nameSet[name] = struct{}{}
	}

	result := make([]string, 0, len(nameSet))
	for name := range nameSet {
		result = append(result, name)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i]) < strings.ToLower(result[j])
	})
	return result
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
		return fmt.Errorf("服务返回为空")
	}
	return nil
}

func (a *App) SetManualGear(gear, level string) bool {
	resp, err := a.sendRequest(ipc.ReqSetManualGear, ipc.SetManualGearParams{Gear: gear, Level: level})
	if err != nil || resp == nil || !resp.Success {
		return false
	}
	var success bool
	if err := json.Unmarshal(resp.Data, &success); err != nil {
		logWarn("解析手动档位设置结果失败", "error", err)
		return false
	}
	return success
}

func (a *App) GetAvailableGears() map[string][]GearCommand {
	resp, err := a.sendRequest(ipc.ReqGetAvailableGears, nil)
	if err != nil || resp == nil || !resp.Success {
		return types.GearCommands
	}
	var gears map[string][]GearCommand
	if err := json.Unmarshal(resp.Data, &gears); err != nil {
		logWarn("解析可用档位失败", "error", err)
		return types.GearCommands
	}
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
		return fmt.Errorf("服务返回为空")
	}
	return nil
}

func (a *App) SetGearLight(enabled bool) bool {
	resp, err := a.sendRequest(ipc.ReqSetGearLight, ipc.SetBoolParams{Enabled: enabled})
	if err != nil || resp == nil {
		return false
	}
	var success bool
	if err := json.Unmarshal(resp.Data, &success); err != nil {
		logWarn("解析档位灯设置结果失败", "error", err)
		return false
	}
	return success
}

func (a *App) SetPowerOnStart(enabled bool) bool {
	resp, err := a.sendRequest(ipc.ReqSetPowerOnStart, ipc.SetBoolParams{Enabled: enabled})
	if err != nil || resp == nil {
		return false
	}
	var success bool
	if err := json.Unmarshal(resp.Data, &success); err != nil {
		logWarn("解析上电启动设置结果失败", "error", err)
		return false
	}
	return success
}

func (a *App) SetSmartStartStop(mode string) bool {
	resp, err := a.sendRequest(ipc.ReqSetSmartStartStop, ipc.SetStringParams{Value: mode})
	if err != nil || resp == nil {
		return false
	}
	var success bool
	if err := json.Unmarshal(resp.Data, &success); err != nil {
		logWarn("解析智能启停设置结果失败", "error", err)
		return false
	}
	return success
}

func (a *App) SetBrightness(percentage int) bool {
	resp, err := a.sendRequest(ipc.ReqSetBrightness, ipc.SetIntParams{Value: percentage})
	if err != nil || resp == nil {
		return false
	}
	var success bool
	if err := json.Unmarshal(resp.Data, &success); err != nil {
		logWarn("解析亮度设置结果失败", "error", err)
		return false
	}
	return success
}

func (a *App) SetRGBMode(params ipc.SetRGBModeParams) bool {
	resp, err := a.sendRequest(ipc.ReqSetRGBMode, params)
	if err != nil || resp == nil {
		return false
	}
	var success bool
	if err := json.Unmarshal(resp.Data, &success); err != nil {
		logWarn("解析 RGB 模式设置结果失败", "error", err)
		return false
	}
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
	if err := json.Unmarshal(resp.Data, &temp); err != nil {
		logWarn("解析温度数据失败", "error", err)
		a.mutex.RLock()
		defer a.mutex.RUnlock()
		return a.currentTemp
	}
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

func (a *App) getAutostartManager() *autostart.Manager {
	if a.autostartManager == nil {
		a.autostartManager = autostart.NewManager(guiLogger, config.GetInstallDir())
	}
	return a.autostartManager
}

func (a *App) SetWindowsAutoStart(enable bool) error {
	return a.getAutostartManager().SetWindowsAutoStart(enable)
}

func (a *App) CheckWindowsAutoStart() bool {
	return a.getAutostartManager().CheckWindowsAutoStart()
}

func (a *App) SetMonitorAutoStart(enable bool) error {
	cfg, _ := a.getMonitorDecisionConfig()
	if enable != cfg.MonitorAutoStart {
		return fmt.Errorf("monitorAutoStart 已改为联动值，当前应为 %t", cfg.MonitorAutoStart)
	}
	a.syncMonitorAutoStartFromConfig("api-set-monitor-autostart")
	return nil
}

func (a *App) CheckMonitorAutoStart() bool {
	cfg, _ := a.getMonitorDecisionConfig()
	return cfg.MonitorAutoStart
}

func (a *App) ShowWindow() {
	if a.ctx == nil {
		return
	}
	if !a.showWindowInFlight.CompareAndSwap(false, true) {
		logDebug("ShowWindow 请求已在处理中，忽略重复触发")
		return
	}
	defer a.showWindowInFlight.Store(false)

	a.mutex.RLock()
	wasVisible := a.windowVisible
	a.mutex.RUnlock()
	logInfo("ShowWindow 调用", "previous_visible", wasVisible)

	a.mutex.Lock()
	a.windowVisible = true
	a.mutex.Unlock()

	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	runtime.WindowSetAlwaysOnTop(a.ctx, false)
	logInfo("ShowWindow 已执行 WindowShow/AlwaysOnTop 刷新")

	runtime.EventsEmit(a.ctx, "window-shown", nil)
	logInfo("ShowWindow 已发送 window-shown 事件")
}

func (a *App) HideWindow() {
	if a.ctx != nil {
		a.mutex.RLock()
		wasVisible := a.windowVisible
		a.mutex.RUnlock()
		logInfo("HideWindow 调用", "previous_visible", wasVisible)
		a.mutex.Lock()
		a.windowVisible = false
		a.mutex.Unlock()
		runtime.EventsEmit(a.ctx, "window-hidden", nil)
		runtime.WindowHide(a.ctx)
	}
}

func (a *App) QuitApp() {
	logInfo("控制台请求退出")
	if a.ipcClient != nil {
		a.ipcClient.Close()
	}
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}

	go func() {
		time.Sleep(500 * time.Millisecond)
		logInfo("控制台已退出")
		if guiLogger != nil {
			guiLogger.Close()
		}
		os.Exit(0)
	}()
}

// RestartCoreService 重启核心服务
func (a *App) RestartCoreService() bool {
	logInfo("控制台请求重启核心服务")
	resp, err := a.sendRequest(ipc.ReqRestartService, nil)
	if err != nil {
		logError("发送重启核心服务请求失败", "error", err)
		return false
	} else if resp != nil && resp.Success {
		a.mutex.Lock()
		a.coreRunning = true
		a.mutex.Unlock()
		logInfo("核心服务重启请求已发送")
		return true
	} else {
		logWarn("重启核心服务请求未成功")
		return false
	}
}

// StopCoreService 停止核心服务
func (a *App) StopCoreService() bool {
	logInfo("控制台请求停止核心服务")
	resp, err := a.sendRequest(ipc.ReqStopService, nil)
	if err != nil {
		logError("发送停止核心服务请求失败", "error", err)
		return false
	} else if resp != nil && resp.Success {
		a.mutex.Lock()
		a.coreRunning = false
		a.isConnected = false
		a.mutex.Unlock()
		logInfo("核心服务已切换为暂停态")
		return true
	} else {
		logWarn("停止核心服务请求未成功")
		return false
	}
}

func (a *App) ResumeCoreService() bool {
	logInfo("控制台请求恢复核心服务")
	resp, err := a.sendRequest(ipc.ReqRestartService, nil)
	if err != nil {
		logError("发送恢复核心服务请求失败", "error", err)
		return false
	}
	if resp == nil || !resp.Success {
		logWarn("恢复核心服务请求未成功")
		return false
	}
	a.mutex.Lock()
	a.coreRunning = true
	a.mutex.Unlock()
	logInfo("核心服务已恢复运行")
	return true
}

func (a *App) ToggleCoreService() bool {
	a.mutex.RLock()
	running := a.coreRunning
	a.mutex.RUnlock()
	if running {
		a.StopCoreService()
		return false
	}
	a.ResumeCoreService()
	return true
}

func (a *App) TestTemperatureReading() TemperatureData {
	resp, err := a.sendRequest(ipc.ReqTestTemperatureReading, nil)
	if err != nil || resp == nil {
		return TemperatureData{}
	}
	var temp TemperatureData
	if err := json.Unmarshal(resp.Data, &temp); err != nil {
		logWarn("解析测试温度结果失败", "error", err)
		return TemperatureData{}
	}
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
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		logWarn("解析桥接程序测试结果失败", "error", err)
		return BridgeTemperatureData{Success: false, Error: err.Error()}
	}
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
	if err := json.Unmarshal(resp.Data, &status); err != nil {
		logWarn("解析桥接程序状态失败", "error", err)
		return map[string]any{"error": err.Error()}
	}
	return status
}

// SendWindowsNotification 发送 Windows Toast 系统通知。
// title 为通知标题，message 为通知正文。
func (a *App) SendWindowsNotification(title, message string) error {
	return notification.Send("", title, message)
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
	if err := json.Unmarshal(resp.Data, &info); err != nil {
		logWarn("解析调试信息失败", "error", err)
		return map[string]any{"error": err.Error()}
	}
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

func (a *App) SetHotkeyEditMode(enabled bool) error {
	resp, err := a.sendRequest(ipc.ReqSetHotkeyEditMode, ipc.SetBoolParams{Enabled: enabled})
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
	args := []any{"component", "frontend", "source", source, "message", message}
	if stack != "" {
		args = append(args, "stack", stack)
	}
	switch level {
	case "debug":
		guiLogger.Debug("前端日志", args...)
	case "warn":
		guiLogger.Warn("前端日志", args...)
	case "crash", "error":
		guiLogger.Error("前端日志", args...)
	default:
		guiLogger.Info("前端日志", args...)
	}
}

// startConnectionHealthCheck 启动连接健康检查
func (a *App) startConnectionHealthCheck() {
	logInfo("启动核心服务健康检查")

	baseInterval := 3 * time.Second     // 基础探测频率
	maxInterval := 30 * time.Second     // 最大探测频率
	maxRetryDuration := 5 * time.Minute // 最大重试时长
	currentInterval := baseInterval
	retryStartTime := time.Now()
	retryStopped := false

	for {
		a.ensureMonitorAgentRunningIfNeeded()

		if !a.ipcClient.IsConnected() {
			// 检查重试时长
			if !retryStopped && time.Since(retryStartTime) > maxRetryDuration {
				logWarn("健康检查: 超过5分钟仍无法连接核心服务，停止重试")
				if a.ctx != nil {
					runtime.EventsEmit(a.ctx, "core-service-error", "核心服务长时间无法连接，请检查服务状态")
				}
				retryStopped = true
			}

			if retryStopped {
				// 维持低频探测
				time.Sleep(30 * time.Second)
				continue
			}

			logInfo("健康检查: 检测到核心服务离线，开始重连")

			if err := a.ipcClient.Connect(); err == nil {
				logInfo("健康检查: 核心服务重连成功")
				currentInterval = baseInterval // 重置探测频率
				retryStartTime = time.Now()    // 重置重试起点
				retryStopped = false           // 重置停止标记
			} else {
				// 连接失败时推送状态
				if a.ctx != nil {
					runtime.EventsEmit(a.ctx, "core-service-error", "核心服务已停止，正在等待服务启动...")
					runtime.EventsEmit(a.ctx, "device-disconnected", nil)
				}

				// 指数退避延长探测间隔
				currentInterval *= 2
				if currentInterval > maxInterval {
					currentInterval = maxInterval
				}
				logDebug("健康检查重连失败", "next_probe_interval", currentInterval)
			}
		} else {
			// 连接正常时发送心跳
			resp, err := a.sendRequest(ipc.ReqPing, nil)
			if err != nil || resp == nil || !resp.Success {
				logError("健康检查: 心跳失败，判定管道异常并主动断开")
				a.ipcClient.Close()
				currentInterval = baseInterval // 准备快速重连
				retryStartTime = time.Now()    // 重置重试起点
				retryStopped = false           // 重置停止标记
			} else {
				currentInterval = baseInterval // 保持正常探测频率
				retryStartTime = time.Now()    // 重置重试起点
				retryStopped = false           // 重置停止标记
			}
		}

		// 统一休眠
		time.Sleep(currentInterval)
	}
}
