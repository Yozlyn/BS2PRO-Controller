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
	suppressWindow             time.Duration
}

func NewManager(enabled func() bool, sender func(Request)) *Manager {
	return &Manager{
		enabled:        enabled,
		sender:         sender,
		lastSent:       make(map[string]time.Time),
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
		if last, ok := m.lastSent[req.DedupKey]; ok && time.Since(last) < m.suppressWindow {
			return
		}
		m.lastSent[req.DedupKey] = time.Now()
	}
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
