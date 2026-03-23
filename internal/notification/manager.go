package notification

import (
	"fmt"
	"sync"
	"time"
)

const (
	TypeDeviceDisconnected    = "device_disconnected"
	TypeDeviceReconnected     = "device_reconnected"
	TypeConfigImportCompleted = "config_import_completed"
	TypeConfigExportCompleted = "config_export_completed"
	TypeServiceReconnected    = "service_reconnected"
	TypeRGBModeChanged        = "rgb_mode_changed"
	TypeProcessSwitchChanged  = "process_switch_changed"
	TypeAutoControlChanged    = "auto_control_changed"
)

type Request struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Title      string         `json:"title"`
	Message    string         `json:"message"`
	Level      string         `json:"level"`
	DedupKey   string         `json:"dedupKey"`
	OccurredAt string         `json:"occurredAt"`
	Payload    map[string]any `json:"payload,omitempty"`
}

type Manager struct {
	mu                         sync.Mutex
	enabled                    func() bool
	sender                     func(Request)
	deviceDisconnectedNotified bool
	lastSent                   map[string]time.Time
	pending                    map[string]Request
	timers                     map[string]*time.Timer
	suppressWindow             time.Duration
}

func NewManager(enabled func() bool, sender func(Request)) *Manager {
	return &Manager{
		enabled:        enabled,
		sender:         sender,
		lastSent:       make(map[string]time.Time),
		pending:        make(map[string]Request),
		timers:         make(map[string]*time.Timer),
		suppressWindow: 3 * time.Second,
	}
}

func (m *Manager) OnDeviceDisconnected() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.isEnabled() || m.deviceDisconnectedNotified {
		return
	}
	m.deviceDisconnectedNotified = true
	m.emitLocked(newRequest(TypeDeviceDisconnected, "设备已断开", "检测不到风扇设备连接，系统将自动重试", "device_disconnected", nil))
}

func (m *Manager) OnDeviceReconnected() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.isEnabled() || !m.deviceDisconnectedNotified {
		return
	}
	m.deviceDisconnectedNotified = false
	m.emitLocked(newRequest(TypeDeviceReconnected, "设备已恢复连接", "风扇设备已重新连接并恢复控制", "device_reconnected", nil))
}

func (m *Manager) OnConfigImportCompleted(profileCount int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.isEnabled() {
		return
	}
	m.emitLocked(newRequest(TypeConfigImportCompleted, "配置导入完成", fmt.Sprintf("新的风扇配置已成功应用，共导入 %d 组配置", profileCount), "config_import_completed", map[string]any{"profileCount": profileCount}))
}

func (m *Manager) OnConfigExportCompleted(profileCount int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.isEnabled() {
		return
	}
	m.emitLocked(newRequest(TypeConfigExportCompleted, "配置导出完成", fmt.Sprintf("配置文件已成功导出，共导出 %d 组配置", profileCount), "config_export_completed", map[string]any{"profileCount": profileCount}))
}

func (m *Manager) OnServiceReconnected() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.isEnabled() {
		return
	}
	m.emitLocked(newRequest(TypeServiceReconnected, "BS2PRO 后台服务已恢复", "快捷键与控制功能已重新连接", "service_reconnected", nil))
}

func (m *Manager) OnRGBModeChanged(modeName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.isEnabled() {
		return
	}
	m.emitLocked(newRequest(TypeRGBModeChanged, "RGB 灯光模式已切换", fmt.Sprintf("当前模式：%s", modeName), "rgb_mode_changed", map[string]any{"mode": modeName}))
}

func (m *Manager) OnProcessSwitchChanged(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.isEnabled() {
		return
	}
	title := "进程联动已关闭"
	message := "快捷键已关闭进程联动"
	if enabled {
		title = "进程联动已开启"
		message = "快捷键已开启进程联动"
	}
	m.emitLocked(newRequest(TypeProcessSwitchChanged, title, message, "process_switch_changed", map[string]any{"enabled": enabled}))
}

func (m *Manager) OnAutoControlChanged(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.isEnabled() {
		return
	}
	title := "智能变频已关闭"
	message := "快捷键已关闭智能变频"
	if enabled {
		title = "智能变频已开启"
		message = "快捷键已开启智能变频"
	}
	m.emitLocked(newRequest(TypeAutoControlChanged, title, message, "auto_control_changed", map[string]any{"enabled": enabled}))
}

func (m *Manager) ResetDeviceDisconnectState() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deviceDisconnectedNotified = false
}

func (m *Manager) isEnabled() bool {
	return m.enabled == nil || m.enabled()
}

func (m *Manager) emitLocked(req Request) {
	if req.DedupKey != "" {
		m.emitDebouncedLocked(req)
		return
	}
	if m.sender != nil {
		m.sender(req)
	}
}

func (m *Manager) emitDebouncedLocked(req Request) {
	now := time.Now()
	last, sent := m.lastSent[req.DedupKey]
	if !sent || now.Sub(last) >= m.suppressWindow {
		m.lastSent[req.DedupKey] = now
		delete(m.pending, req.DedupKey)
		if timer := m.timers[req.DedupKey]; timer != nil {
			timer.Stop()
			delete(m.timers, req.DedupKey)
		}
		if m.sender != nil {
			m.sender(req)
		}
		return
	}

	m.pending[req.DedupKey] = req
	delay := m.suppressWindow - now.Sub(last)
	if timer := m.timers[req.DedupKey]; timer != nil {
		timer.Reset(delay)
		return
	}
	m.timers[req.DedupKey] = time.AfterFunc(delay, func() {
		m.flushPending(req.DedupKey)
	})
}

func (m *Manager) flushPending(dedupKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	req, ok := m.pending[dedupKey]
	if !ok {
		delete(m.timers, dedupKey)
		return
	}
	delete(m.pending, dedupKey)
	delete(m.timers, dedupKey)
	if !m.isEnabled() {
		return
	}
	m.lastSent[dedupKey] = time.Now()
	if m.sender != nil {
		m.sender(req)
	}
}

func newRequest(typ, title, message, dedupKey string, payload map[string]any) Request {
	return Request{
		ID:         fmt.Sprintf("%d-%s", time.Now().UnixNano(), typ),
		Type:       typ,
		Title:      title,
		Message:    message,
		Level:      "info",
		DedupKey:   dedupKey,
		OccurredAt: time.Now().Format(time.RFC3339),
		Payload:    payload,
	}
}
