package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TIANLI0/BS2PRO-Controller/internal/config"
	"github.com/TIANLI0/BS2PRO-Controller/internal/ipc"
	"github.com/TIANLI0/BS2PRO-Controller/internal/logger"
	"github.com/TIANLI0/BS2PRO-Controller/internal/notification"
	"github.com/TIANLI0/BS2PRO-Controller/internal/platformutil"
	"github.com/TIANLI0/BS2PRO-Controller/internal/tray"
	"github.com/TIANLI0/BS2PRO-Controller/internal/types"
	"golang.org/x/sys/windows"
)

var (
	monitorKernel32       = windows.NewLazySystemDLL("kernel32.dll")
	monitorUser32         = windows.NewLazySystemDLL("user32.dll")
	procConsoleWindow     = monitorKernel32.NewProc("GetConsoleWindow")
	procMonitorShowWindow = monitorUser32.NewProc("ShowWindow")
)

const hideWindowCmd = uintptr(0)

//go:embed tray.ico
var trayIconData []byte

var globalHotkeys = newGlobalHotkeyAgent()
var monitorInstance = createSingleInstanceGuard()
var monitorTrayState = newMonitorTrayState()
var monitorTrayManager *tray.Manager
var monitorLogger *logger.CustomLogger
var hotkeyEditMode bool
var lastGUIShowRequest int64

const serviceNotifyWindow = 3 * time.Second

func init() {
	initMonitorLogger()
}

func monitorDebug(msg string, args ...any) {
	if monitorLogger != nil {
		monitorLogger.Debug(msg, args...)
	}
}

func monitorInfo(msg string, args ...any) {
	if monitorLogger != nil {
		monitorLogger.Info(msg, args...)
	}
}

func monitorWarn(msg string, args ...any) {
	if monitorLogger != nil {
		monitorLogger.Warn(msg, args...)
	}
}

func monitorError(msg string, args ...any) {
	if monitorLogger != nil {
		monitorLogger.Error(msg, args...)
	}
}

func setMonitorDebugMode(enabled bool) {
	if monitorLogger != nil {
		monitorLogger.SetDebugMode(enabled)
	}
}

func initMonitorLogger() {
	customLogger, err := logger.NewCustomLoggerWithOptions(false, filepath.Dir(config.GetLogDir()), "monitor", logger.Options{
		Format:  logger.FormatText,
		Console: false,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "init monitor logger failed: %v\n", err)
		return
	}
	customLogger.CleanOldLogs()
	monitorLogger = customLogger
}

func main() {
	hideConsoleWindow()

	if !monitorInstance.Acquire() {
		monitorDebug("monitor instance already running, exit")
		return
	}
	defer monitorInstance.Release()

	if err := notification.EnsureCurrentProcessAppID(""); err != nil {
		monitorWarn("ensure process app id failed", "error", err)
	}
	if err := notification.EnsureMonitorStartMenuShortcut(); err != nil {
		monitorWarn("ensure monitor shortcut failed", "error", err)
	}
	configManager := config.NewManager(config.GetInstallDir(), nil)
	startupCfg := configManager.Load(false)
	syncTrayEnabled(startupCfg.TrayEnabled)

	client := ipc.NewClient(nil)
	client.SetRole(ipc.RoleMonitorAgent)
	client.SetEventHandler(handleEvent)
	serviceState := newServiceStateNotifier()

	for {
		if err := client.Connect(); err != nil {
			serviceState.MarkDisconnected()
			monitorWarn("connect core failed", "error", err)
			time.Sleep(3 * time.Second)
			continue
		}

		serviceState.MarkConnected()
		if _, err := client.SendRequest(ipc.ReqRegisterClient, ipc.RegisterClientParams{Role: ipc.RoleMonitorAgent}); err != nil {
			monitorWarn("register monitor failed", "error", err)
		} else {
			monitorInfo("register monitor success")
		}
		refreshMonitorTrayState(client)
		refreshGlobalHotkeys(client)

		for client.IsConnected() {
			refreshMonitorTrayState(client)
			processName := strings.TrimSpace(getForegroundProcessName())
			if processName != "" {
				if _, err := client.SendRequest(ipc.ReqReportForegroundProcess, ipc.ReportForegroundProcessParams{
					ProcessName: processName,
					ReportedAt:  time.Now().Format(time.RFC3339),
				}); err != nil {
					monitorWarn("report foreground process failed", "error", err)
					break
				}
			}
			time.Sleep(2 * time.Second)
		}

		client.Close()
		monitorTrayState.SetDisconnected()
		globalHotkeys.Clear()
		time.Sleep(1 * time.Second)
	}
}

func hideConsoleWindow() {
	hwnd, _, _ := procConsoleWindow.Call()
	if hwnd == 0 {
		return
	}
	procMonitorShowWindow.Call(hwnd, hideWindowCmd)
}

type monitorTrayStateStore struct {
	mu     sync.RWMutex
	status tray.Status
}

func newMonitorTrayState() *monitorTrayStateStore {
	return &monitorTrayStateStore{}
}

func (s *monitorTrayStateStore) SetStatus(status tray.Status) {
	s.mu.Lock()
	s.status = status
	s.mu.Unlock()
}

func (s *monitorTrayStateStore) GetStatus() tray.Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *monitorTrayStateStore) SetDisconnected() {
	s.mu.Lock()
	s.status.CoreRunning = false
	s.status.Connected = false
	s.status.CPUTemp = 0
	s.status.GPUTemp = 0
	s.status.CurrentRPM = 0
	s.mu.Unlock()
}

type monitorTrayLoggerAdapter struct{}

func (l *monitorTrayLoggerAdapter) Info(msg any, args ...any) { monitorInfo(fmt.Sprint(msg), args...) }
func (l *monitorTrayLoggerAdapter) Error(msg any, args ...any) {
	monitorError(fmt.Sprint(msg), args...)
}
func (l *monitorTrayLoggerAdapter) Debug(msg any, args ...any) {
	monitorDebug(fmt.Sprint(msg), args...)
}
func (l *monitorTrayLoggerAdapter) Warn(msg any, args ...any) { monitorWarn(fmt.Sprint(msg), args...) }
func (l *monitorTrayLoggerAdapter) Close()                    {}
func (l *monitorTrayLoggerAdapter) CleanOldLogs()             {}
func (l *monitorTrayLoggerAdapter) SetDebugMode(enabled bool) { setMonitorDebugMode(enabled) }
func (l *monitorTrayLoggerAdapter) GetLogDir() string         { return config.GetLogDir() }

func initMonitorTray() {
	if monitorTrayManager != nil && monitorTrayManager.IsInitialized() {
		return
	}
	manager := tray.NewManager(&monitorTrayLoggerAdapter{}, trayIconData)
	manager.SetCallbacks(
		func() { showOrLaunchGUI() },
		func() { quitGUI() },
		func() { triggerCoreRestart() },
		func() bool { return toggleCorePaused() },
		func() bool { return toggleAutoControl() },
		func() tray.Status { return monitorTrayState.GetStatus() },
	)
	monitorTrayManager = manager
	manager.Init()
}

func syncTrayEnabled(enabled bool) {
	if enabled {
		initMonitorTray()
		return
	}
	if monitorTrayManager != nil && monitorTrayManager.IsInitialized() {
		monitorInfo("tray disabled by config, quitting tray manager")
		monitorTrayManager.Quit()
	}
}

func showOrLaunchGUI() {
	now := time.Now().UnixMilli()
	last := atomic.LoadInt64(&lastGUIShowRequest)
	monitorInfo("show or launch gui requested", "now", now, "last", last, "delta_ms", now-last)
	if last != 0 && now-last < 1200 {
		monitorInfo("show or launch gui ignored by debounce", "delta_ms", now-last)
		return
	}
	atomic.StoreInt64(&lastGUIShowRequest, now)
	monitorInfo("show or launch gui triggered: launch gui directly")
	launchGUI()
}

func launchGUI() {
	guiPath := filepath.Join(config.GetInstallDir(), "BS2PRO-Controller.exe")
	monitorInfo("launch gui start", "path", guiPath)
	if _, err := os.Stat(guiPath); err != nil {
		monitorWarn("launch gui stat failed", "path", guiPath, "error", err)
		return
	}
	cmd := exec.Command(guiPath)
	cmd.Dir = filepath.Dir(guiPath)
	platformutil.HideCommandWindow(cmd)
	if err := cmd.Start(); err != nil {
		monitorError("launch gui failed", "path", guiPath, "error", err)
		return
	}
	monitorInfo("launch gui started", "path", guiPath, "pid", cmd.Process.Pid)
}

func quitGUI() {
	checkCmd := exec.Command("tasklist", "/FI", "IMAGENAME eq BS2PRO-Controller.exe")
	platformutil.HideCommandWindow(checkCmd)
	if out, err := checkCmd.Output(); err == nil {
		monitorInfo("quit gui precheck", "process_found", strings.Contains(strings.ToLower(string(out)), "bs2pro-controller.exe"))
	} else {
		monitorWarn("quit gui precheck failed", "error", err)
	}
	cmd := exec.Command("taskkill", "/F", "/IM", "BS2PRO-Controller.exe")
	platformutil.HideCommandWindow(cmd)
	monitorInfo("quit gui taskkill start")
	if err := cmd.Run(); err != nil {
		monitorWarn("quit gui failed", "error", err)
		return
	}
	monitorInfo("quit gui taskkill success")
}

func triggerCoreRestart() {
	client := ipc.NewClient(nil)
	if err := client.Connect(); err != nil {
		monitorWarn("restart core connect failed", "error", err)
		return
	}
	defer client.Close()
	if _, err := client.SendRequest(ipc.ReqRestartService, nil); err != nil {
		monitorWarn("restart core failed", "error", err)
	}
}

func toggleCorePaused() bool {
	client := ipc.NewClient(nil)
	if err := client.Connect(); err != nil {
		monitorWarn("toggle core connect failed", "error", err)
		return monitorTrayState.GetStatus().CoreRunning
	}
	defer client.Close()
	status := monitorTrayState.GetStatus()
	requestType := ipc.ReqStopService
	nextRunning := false
	if !status.CoreRunning {
		requestType = ipc.ReqRestartService
		nextRunning = true
	}
	if _, err := client.SendRequest(requestType, nil); err != nil {
		monitorWarn("toggle core failed", "request_type", requestType, "error", err)
		return status.CoreRunning
	}
	status.CoreRunning = nextRunning
	if !nextRunning {
		status.Connected = false
		status.CPUTemp = 0
		status.GPUTemp = 0
		status.CurrentRPM = 0
	}
	monitorTrayState.SetStatus(status)
	return nextRunning
}

func toggleAutoControl() bool {
	client := ipc.NewClient(nil)
	if err := client.Connect(); err != nil {
		monitorWarn("toggle auto control connect failed", "error", err)
		return monitorTrayState.GetStatus().AutoControlState
	}
	defer client.Close()
	status := monitorTrayState.GetStatus()
	next := !status.AutoControlState
	if _, err := client.SendRequest(ipc.ReqSetAutoControl, ipc.SetAutoControlParams{Enabled: next}); err != nil {
		monitorWarn("toggle auto control failed", "enabled", next, "error", err)
		return status.AutoControlState
	}
	status.AutoControlState = next
	monitorTrayState.SetStatus(status)
	return next
}

func refreshMonitorTrayState(client *ipc.Client) {
	if client == nil || !client.IsConnected() {
		monitorTrayState.SetDisconnected()
		return
	}
	statusResp, err := client.SendRequest(ipc.ReqGetDeviceStatus, nil)
	if err != nil || statusResp == nil || !statusResp.Success {
		monitorTrayState.SetDisconnected()
		return
	}
	cfgResp, err := client.SendRequest(ipc.ReqGetConfig, nil)
	if err != nil || cfgResp == nil || !cfgResp.Success {
		return
	}
	var deviceStatus map[string]any
	var cfg types.AppConfig
	if err := json.Unmarshal(statusResp.Data, &deviceStatus); err != nil {
		return
	}
	if err := json.Unmarshal(cfgResp.Data, &cfg); err != nil {
		return
	}
	status := monitorTrayState.GetStatus()
	status.CoreRunning = true
	status.AutoControlState = cfg.AutoControl
	if connected, ok := deviceStatus["connected"].(bool); ok {
		status.Connected = connected
	}
	if paused, ok := deviceStatus["paused"].(bool); ok {
		status.CoreRunning = !paused
	}
	if temp, ok := deviceStatus["temperature"].(map[string]any); ok {
		if cpu, ok := temp["cpuTemp"].(float64); ok {
			status.CPUTemp = int(cpu)
		}
		if gpu, ok := temp["gpuTemp"].(float64); ok {
			status.GPUTemp = int(gpu)
		}
	}
	if currentData, ok := deviceStatus["currentData"].(map[string]any); ok {
		if rpm, ok := currentData["currentRpm"].(float64); ok {
			status.CurrentRPM = uint16(rpm)
		}
	}
	if !status.CoreRunning {
		status.Connected = false
	}
	monitorTrayState.SetStatus(status)
}

type serviceStateNotifier struct {
	mu         sync.Mutex
	lastState  string
	pending    string
	timer      *time.Timer
	lastSentAt time.Time
}

func newServiceStateNotifier() *serviceStateNotifier {
	return &serviceStateNotifier{lastState: "connected"}
}

func (n *serviceStateNotifier) MarkDisconnected() {
	n.mark("disconnected")
}

func (n *serviceStateNotifier) MarkConnected() {
	n.mark("connected")
}

func (n *serviceStateNotifier) mark(state string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.pending == state {
		return
	}
	n.pending = state
	if n.timer != nil {
		n.timer.Stop()
	}
	n.timer = time.AfterFunc(serviceNotifyWindow, func() {
		n.flush(state)
	})
}

func (n *serviceStateNotifier) flush(expected string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.pending != expected {
		return
	}
	n.pending = ""
	n.timer = nil
	if n.lastState == expected && time.Since(n.lastSentAt) < serviceNotifyWindow {
		return
	}
	n.lastState = expected
	n.lastSentAt = time.Now()
	switch expected {
	case "disconnected":
		_ = notification.Send("", "BS2PRO 后台服务已断开", "控制功能暂时不可用，系统正在等待恢复")
	case "connected":
		_ = notification.Send("", "BS2PRO 后台服务已恢复", "快捷键与控制功能已重新连接")
	}
}

type singleInstanceGuard struct {
	handle windows.Handle
}

func createSingleInstanceGuard() *singleInstanceGuard {
	return &singleInstanceGuard{}
}

func (g *singleInstanceGuard) Acquire() bool {
	name, err := windows.UTF16PtrFromString("Local\\BS2PRO-Controller-Monitor")
	if err != nil {
		if monitorLogger != nil {
			monitorError("create mutex name failed", "error", err)
		} else {
			fmt.Fprintf(os.Stderr, "create mutex name failed: %v\n", err)
		}
		return true
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		if err == windows.ERROR_ALREADY_EXISTS {
			if handle != 0 {
				windows.CloseHandle(handle)
			}
			return false
		}
		if monitorLogger != nil {
			monitorError("create mutex failed", "error", err)
		} else {
			fmt.Fprintf(os.Stderr, "create mutex failed: %v\n", err)
		}
		return true
	}
	g.handle = handle
	return true
}

func (g *singleInstanceGuard) Release() {
	if g.handle == 0 {
		return
	}
	_ = windows.CloseHandle(g.handle)
	g.handle = 0
	_ = os.ErrClosed
}

func refreshGlobalHotkeys(client *ipc.Client) {
	resp, err := client.SendRequest(ipc.ReqGetConfig, nil)
	if err != nil || resp == nil || !resp.Success {
		monitorWarn("load config for hotkeys failed", "response_nil", resp == nil, "success", resp != nil && resp.Success, "error", err)
		return
	}
	var cfg types.AppConfig
	if err := json.Unmarshal(resp.Data, &cfg); err != nil {
		monitorWarn("parse config for hotkeys failed", "error", err)
		return
	}
	setMonitorDebugMode(cfg.DebugMode)
	if cfg.Hotkeys == nil {
		monitorDebug("refresh global hotkeys with nil config")
	} else {
		monitorDebug("refresh global hotkeys bindings", "enabled", cfg.Hotkeys.Enabled, "global", len(cfg.Hotkeys.Global), "inapp", len(cfg.Hotkeys.InApp))
		for i, binding := range cfg.Hotkeys.Global {
			monitorDebug("global binding", "index", i, "action", binding.Action, "accelerator", binding.Accelerator, "enabled", binding.Enabled, "scope", binding.Scope)
		}
	}
	globalHotkeys.Apply(cfg.Hotkeys, func(action string) {
		handleGlobalAction(client, action)
	})
	conflicts := globalHotkeys.Conflicts()
	globalCount := 0
	if cfg.Hotkeys != nil {
		globalCount = len(cfg.Hotkeys.Global)
	}
	monitorDebug("refresh global hotkeys", "enabled", cfg.Hotkeys != nil && cfg.Hotkeys.Enabled, "global", globalCount, "conflicts", len(conflicts))
	if _, err := client.SendRequest(ipc.ReqReportHotkeyConflicts, conflicts); err != nil {
		monitorWarn("report hotkey conflicts failed", "error", err)
	}
}

func handleGlobalAction(client *ipc.Client, action string) {
	if hotkeyEditMode {
		monitorDebug("hotkey action ignored during edit mode", "action", action)
		return
	}
	if action == "show-main-window" {
		monitorDebug("dispatch global hotkey action locally", "action", action)
		showOrLaunchGUI()
		return
	}
	monitorDebug("dispatch global hotkey action", "action", action)
	if _, err := client.SendRequest(ipc.ReqTriggerHotkeyAction, ipc.TriggerHotkeyActionParams{Action: action}); err != nil {
		monitorWarn("trigger hotkey action failed", "action", action, "error", err)
	}
}

func handleEvent(event ipc.Event) {
	monitorDebug("monitor event received", "type", event.Type, "bytes", len(event.Data))
	if event.Type == ipc.EventConfigUpdate {
		var cfg types.AppConfig
		if err := json.Unmarshal(event.Data, &cfg); err == nil {
			setMonitorDebugMode(cfg.DebugMode)
			syncTrayEnabled(cfg.TrayEnabled)
			if cfg.Hotkeys == nil {
				monitorDebug("config update hotkeys=nil")
			} else {
				monitorDebug("config update hotkeys", "enabled", cfg.Hotkeys.Enabled, "global", len(cfg.Hotkeys.Global))
			}
			globalHotkeys.Apply(cfg.Hotkeys, nil)
		} else {
			monitorWarn("config update parse failed", "error", err)
		}
		return
	}
	if event.Type == ipc.EventHotkeyEditMode {
		var params ipc.SetBoolParams
		if err := json.Unmarshal(event.Data, &params); err == nil {
			hotkeyEditMode = params.Enabled
			monitorInfo("hotkey edit mode changed", "enabled", hotkeyEditMode)
		}
		return
	}
	if event.Type != ipc.EventNotificationRequest {
		return
	}

	var req notification.Request
	if err := json.Unmarshal(event.Data, &req); err != nil {
		monitorWarn("parse notification failed", "error", err)
		return
	}
	monitorDebug("received notification event", "type", req.Type, "title", req.Title)
	if err := notification.Send("", req.Title, req.Message); err != nil {
		monitorWarn("show notification failed", "type", req.Type, "title", req.Title, "error", err)
		return
	}
	monitorDebug("notification shown", "type", req.Type)
}

type globalHotkeyAgent struct {
	mu        sync.Mutex
	handler   func(string)
	bindings  map[int]types.HotkeyBinding
	loopStop  func()
	conflicts []types.HotkeyConflict
	current   *types.HotkeyConfig
}

func newGlobalHotkeyAgent() *globalHotkeyAgent {
	return &globalHotkeyAgent{bindings: map[int]types.HotkeyBinding{}}
}

func (a *globalHotkeyAgent) Apply(cfg *types.HotkeyConfig, handler func(string)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	monitorDebug("globalHotkeyAgent.Apply start", "handler_set", handler != nil, "cfg_nil", cfg == nil)
	if handler != nil {
		a.handler = handler
	}
	if hotkeyConfigEqual(a.current, cfg) {
		monitorInfo("hotkeys unchanged, skip re-register")
		return
	}
	a.clearLocked()
	a.conflicts = nil
	a.current = cloneHotkeyConfig(cfg)
	if cfg == nil || !cfg.Enabled {
		monitorInfo("hotkeys disabled or nil, unregister complete")
		return
	}
	stop, bindings, err := registerGlobalHotkeys(cfg.Global, a.dispatch)
	if err != nil {
		monitorWarn("register global hotkeys failed", "error", err)
		a.conflicts = buildHotkeyConflictsFromError(err)
		return
	}
	a.loopStop = stop
	a.bindings = bindings
	a.conflicts = collectSystemHotkeyConflicts(cfg.Global, bindings)
	monitorInfo("global hotkeys active", "bindings", len(bindings), "conflicts", len(a.conflicts))
}

func (a *globalHotkeyAgent) Conflicts() []types.HotkeyConflict {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := make([]types.HotkeyConflict, len(a.conflicts))
	copy(result, a.conflicts)
	return result
}

func (a *globalHotkeyAgent) dispatch(action string) {
	a.mu.Lock()
	handler := a.handler
	a.mu.Unlock()
	if handler != nil {
		monitorDebug("received WM_HOTKEY action", "action", action)
		handler(action)
	}
}

func (a *globalHotkeyAgent) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.clearLocked()
}

func (a *globalHotkeyAgent) clearLocked() {
	if a.loopStop != nil {
		monitorInfo("clearing existing global hotkey loop")
		a.loopStop()
		a.loopStop = nil
	}
	a.bindings = map[int]types.HotkeyBinding{}
}

func cloneHotkeyConfig(cfg *types.HotkeyConfig) *types.HotkeyConfig {
	if cfg == nil {
		return nil
	}
	clone := &types.HotkeyConfig{Enabled: cfg.Enabled}
	if len(cfg.Global) > 0 {
		clone.Global = append([]types.HotkeyBinding(nil), cfg.Global...)
	}
	if len(cfg.InApp) > 0 {
		clone.InApp = append([]types.HotkeyBinding(nil), cfg.InApp...)
	}
	return clone
}

func hotkeyConfigEqual(a, b *types.HotkeyConfig) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Enabled != b.Enabled || len(a.Global) != len(b.Global) || len(a.InApp) != len(b.InApp) {
		return false
	}
	for i := range a.Global {
		if a.Global[i] != b.Global[i] {
			return false
		}
	}
	for i := range a.InApp {
		if a.InApp[i] != b.InApp[i] {
			return false
		}
	}
	return true
}

func buildHotkeyConflictsFromError(err error) []types.HotkeyConflict {
	if err == nil {
		return nil
	}
	text := err.Error()
	if !strings.Contains(text, ":") {
		return nil
	}
	parts := strings.Split(text, ";")
	conflicts := make([]types.HotkeyConflict, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		open := strings.Index(part, "(")
		close := strings.Index(part, ")")
		colon := strings.LastIndex(part, ":")
		if open < 0 || close < 0 || colon < 0 || close <= open {
			continue
		}
		action := strings.TrimSpace(part[:open])
		accelerator := strings.TrimSpace(part[open+1 : close])
		message := strings.TrimSpace(part[colon+1:])
		conflicts = append(conflicts, types.HotkeyConflict{
			Accelerator: accelerator,
			Scopes:      []string{"global"},
			Actions:     []string{action},
			Source:      "system",
			Message:     message,
		})
	}
	return conflicts
}

func collectSystemHotkeyConflicts(bindings []types.HotkeyBinding, active map[int]types.HotkeyBinding) []types.HotkeyConflict {
	activeActions := map[string]bool{}
	for _, binding := range active {
		activeActions[binding.Action] = true
	}
	conflicts := make([]types.HotkeyConflict, 0)
	for _, binding := range bindings {
		if !binding.Enabled || binding.Accelerator == "" || activeActions[binding.Action] {
			continue
		}
		conflicts = append(conflicts, types.HotkeyConflict{
			Accelerator: binding.Accelerator,
			Scopes:      []string{"global"},
			Actions:     []string{binding.Action},
			Source:      "system",
			Message:     "快捷键已被系统或其他程序占用",
		})
	}
	return conflicts
}
