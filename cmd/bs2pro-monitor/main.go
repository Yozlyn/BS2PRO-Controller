package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/TIANLI0/BS2PRO-Controller/internal/config"
	"github.com/TIANLI0/BS2PRO-Controller/internal/ipc"
	"github.com/TIANLI0/BS2PRO-Controller/internal/notification"
	"github.com/TIANLI0/BS2PRO-Controller/internal/types"
	"golang.org/x/sys/windows"
)

var globalHotkeys = newGlobalHotkeyAgent()
var monitorInstance = createSingleInstanceGuard()
var monitorDebugMode bool
var monitorWriter *os.File
var hotkeyEditMode bool

const serviceNotifyWindow = 3 * time.Second

func init() {
	initMonitorLogger()
}

func monitorLog(format string, args ...any) {
	if monitorDebugMode {
		writeMonitorLog("DEBUG", format, args...)
	}
}

func monitorInfo(format string, args ...any) {
	writeMonitorLog("INFO", format, args...)
}

func setMonitorDebugMode(enabled bool) {
	monitorDebugMode = enabled
}

func initMonitorLogger() {
	path := filepath.Join(filepath.Dir(config.GetLogDir()), "logs", fmt.Sprintf("monitor_%s.log", time.Now().Format("2006-01-02")))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("create monitor log dir failed: %v", err)
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("open monitor log failed: %v", err)
		return
	}
	monitorWriter = file
}

func writeMonitorLog(level, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("\"%s\",\"%s\",\"%s\"\n", level, time.Now().Format("2006-01-02 15:04:05"), escapeMonitorLog(message))
	if monitorWriter != nil {
		if _, err := monitorWriter.WriteString(line); err == nil {
			return
		}
	}
	log.Print(strings.TrimSpace(line))
}

func escapeMonitorLog(message string) string {
	message = strings.ReplaceAll(message, "\r", " ")
	message = strings.ReplaceAll(message, "\n", " ")
	message = strings.ReplaceAll(message, "\"", "'")
	return message
}

func main() {
	if !monitorInstance.Acquire() {
		monitorLog("monitor instance already running, exit")
		return
	}
	defer monitorInstance.Release()

	if err := notification.EnsureCurrentProcessAppID(""); err != nil {
		monitorLog("ensure process app id failed: %v", err)
	}
	if err := notification.EnsureMonitorStartMenuShortcut(); err != nil {
		monitorLog("ensure monitor shortcut failed: %v", err)
	}

	client := ipc.NewClient(nil)
	client.SetRole(ipc.RoleMonitorAgent)
	client.SetEventHandler(handleEvent)
	serviceState := newServiceStateNotifier()

	for {
		if err := client.Connect(); err != nil {
			serviceState.MarkDisconnected()
			monitorLog("connect core failed: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}

		serviceState.MarkConnected()
		if _, err := client.SendRequest(ipc.ReqRegisterClient, ipc.RegisterClientParams{Role: ipc.RoleMonitorAgent}); err != nil {
			monitorInfo("register monitor failed: %v", err)
		} else {
			monitorInfo("register monitor success")
		}
		refreshGlobalHotkeys(client)

		for client.IsConnected() {
			processName := strings.TrimSpace(getForegroundProcessName())
			if processName != "" {
				if _, err := client.SendRequest(ipc.ReqReportForegroundProcess, ipc.ReportForegroundProcessParams{
					ProcessName: processName,
					ReportedAt:  time.Now().Format(time.RFC3339),
				}); err != nil {
					monitorLog("report foreground process failed: %v", err)
					break
				}
			}
			time.Sleep(2 * time.Second)
		}

		client.Close()
		globalHotkeys.Clear()
		time.Sleep(1 * time.Second)
	}
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
		log.Printf("create mutex name failed: %v", err)
		return true
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		log.Printf("create mutex failed: %v", err)
		return true
	}
	if windows.GetLastError() == windows.ERROR_ALREADY_EXISTS {
		windows.CloseHandle(handle)
		return false
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
		monitorLog("load config for hotkeys failed: %v", err)
		return
	}
	var cfg types.AppConfig
	if err := json.Unmarshal(resp.Data, &cfg); err != nil {
		monitorInfo("parse config for hotkeys failed: %v", err)
		return
	}
	setMonitorDebugMode(cfg.DebugMode)
	if cfg.Hotkeys == nil {
		monitorLog("refresh global hotkeys with nil config")
	} else {
		monitorLog("refresh global hotkeys bindings: enabled=%v global=%d inapp=%d", cfg.Hotkeys.Enabled, len(cfg.Hotkeys.Global), len(cfg.Hotkeys.InApp))
		for i, binding := range cfg.Hotkeys.Global {
			monitorLog("global binding[%d]: action=%s accelerator=%s enabled=%v scope=%s", i, binding.Action, binding.Accelerator, binding.Enabled, binding.Scope)
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
	monitorLog("refresh global hotkeys: enabled=%v global=%d conflicts=%d", cfg.Hotkeys != nil && cfg.Hotkeys.Enabled, globalCount, len(conflicts))
	if _, err := client.SendRequest(ipc.ReqReportHotkeyConflicts, conflicts); err != nil {
		monitorInfo("report hotkey conflicts failed: %v", err)
	}
}

func handleGlobalAction(client *ipc.Client, action string) {
	if hotkeyEditMode {
		monitorLog("hotkey action ignored during edit mode: %s", action)
		return
	}
	monitorLog("dispatch global hotkey action: %s", action)
	if _, err := client.SendRequest(ipc.ReqTriggerHotkeyAction, ipc.TriggerHotkeyActionParams{Action: action}); err != nil {
		monitorInfo("trigger hotkey action failed: action=%s err=%v", action, err)
	}
}

func handleEvent(event ipc.Event) {
	monitorLog("monitor event received: type=%s bytes=%d", event.Type, len(event.Data))
	if event.Type == ipc.EventConfigUpdate {
		var cfg types.AppConfig
		if err := json.Unmarshal(event.Data, &cfg); err == nil {
			setMonitorDebugMode(cfg.DebugMode)
			if cfg.Hotkeys == nil {
				monitorLog("config update hotkeys=nil")
			} else {
				monitorLog("config update hotkeys: enabled=%v global=%d", cfg.Hotkeys.Enabled, len(cfg.Hotkeys.Global))
			}
			globalHotkeys.Apply(cfg.Hotkeys, nil)
		} else {
			monitorLog("config update parse failed: %v", err)
		}
		return
	}
	if event.Type == ipc.EventHotkeyEditMode {
		var params ipc.SetBoolParams
		if err := json.Unmarshal(event.Data, &params); err == nil {
			hotkeyEditMode = params.Enabled
			monitorInfo("hotkey edit mode changed: enabled=%v", hotkeyEditMode)
		}
		return
	}
	if event.Type != ipc.EventNotificationRequest {
		return
	}

	var req notification.Request
	if err := json.Unmarshal(event.Data, &req); err != nil {
		monitorInfo("parse notification failed: %v", err)
		return
	}
	monitorLog("received notification event: type=%s title=%s", req.Type, req.Title)
	if err := notification.Send("", req.Title, req.Message); err != nil {
		monitorInfo("show notification failed: %v", err)
		return
	}
	monitorLog("notification shown: type=%s", req.Type)
}

type globalHotkeyAgent struct {
	mu       sync.Mutex
	handler  func(string)
	bindings map[int]types.HotkeyBinding
	loopStop func()
	conflicts []types.HotkeyConflict
	current  *types.HotkeyConfig
}

func newGlobalHotkeyAgent() *globalHotkeyAgent {
	return &globalHotkeyAgent{bindings: map[int]types.HotkeyBinding{}}
}

func (a *globalHotkeyAgent) Apply(cfg *types.HotkeyConfig, handler func(string)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	monitorLog("globalHotkeyAgent.Apply start: handlerSet=%v cfgNil=%v", handler != nil, cfg == nil)
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
		monitorInfo("register global hotkeys failed: %v", err)
		a.conflicts = buildHotkeyConflictsFromError(err)
		return
	}
	a.loopStop = stop
	a.bindings = bindings
	a.conflicts = collectSystemHotkeyConflicts(cfg.Global, bindings)
	monitorInfo("global hotkeys active=%d conflicts=%d", len(bindings), len(a.conflicts))
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
		monitorLog("received WM_HOTKEY action: %s", action)
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
