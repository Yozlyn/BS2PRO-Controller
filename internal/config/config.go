// Package config 提供配置管理功能
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"

	"github.com/TIANLI0/BS2PRO-Controller/internal/types"
)

// Manager 配置管理器
type Manager struct {
	config     types.AppConfig
	installDir string
	logger     types.Logger
}

// NewManager 创建新的配置管理器
func NewManager(installDir string, logger types.Logger) *Manager {
	return &Manager{
		installDir: installDir,
		logger:     logger,
	}
}

// Load 加载配置
func (m *Manager) Load(isAutoStart bool) types.AppConfig {
	// 优先尝试从默认目录加载配置
	defaultConfigDir := m.GetDefaultConfigDir()
	defaultConfigPath := filepath.Join(defaultConfigDir, "config.json")

	installConfigPath := filepath.Join(m.installDir, "config", "config.json")

	m.logDebug("尝试从默认目录加载配置", "path", defaultConfigPath, "source", "default")

	// 先尝试从默认目录加载
	if loaded, changed := m.tryLoadFromPath(defaultConfigPath); loaded {
		m.config.ConfigPath = defaultConfigPath
		if changed {
			if err := m.Save(); err != nil {
				m.logError("保存迁移后的配置失败", "path", defaultConfigPath, "error", err)
			}
		}
		m.logInfo("配置加载成功", "path", defaultConfigPath, "source", "default")
		return m.config
	}

	m.logDebug("从默认目录加载配置失败，尝试从安装目录加载", "path", installConfigPath, "source", "install")

	// 默认目录失败，尝试从安装目录加载
	if loaded, changed := m.tryLoadFromPath(installConfigPath); loaded {
		m.config.ConfigPath = installConfigPath
		if changed {
			if err := m.Save(); err != nil {
				m.logError("保存迁移后的配置失败", "path", installConfigPath, "error", err)
			}
		}
		m.logInfo("配置加载成功", "path", installConfigPath, "source", "install")
		return m.config
	}

	m.logError("所有配置目录加载失败，使用默认配置", "path", defaultConfigPath, "source", "default")

	m.config = types.GetDefaultConfig(isAutoStart)
	m.config.ConfigPath = defaultConfigPath
	if err := m.Save(); err != nil {
		m.logError("保存默认配置失败", "path", defaultConfigPath, "error", err)
	}

	return m.config
}

// tryLoadFromPath 尝试从指定路径加载配置
func (m *Manager) tryLoadFromPath(configPath string) (bool, bool) {
	if _, err := os.Stat(configPath); err != nil {
		m.logDebug("配置文件不存在", "path", configPath)
		return false, false
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		m.logError("读取配置文件失败", "path", configPath, "error", err)
		return false, false
	}

	var raw map[string]json.RawMessage
	hasNotificationsEnabled := false
	hasTrayEnabled := false
	if err := json.Unmarshal(data, &raw); err == nil {
		_, hasNotificationsEnabled = raw["notificationsEnabled"]
		_, hasTrayEnabled = raw["trayEnabled"]
	}

	var config types.AppConfig
	if err := json.Unmarshal(data, &config); err != nil {
		m.logError("解析配置文件失败", "path", configPath, "error", err)
		return false, false
	}
	original := config
	changed := false
	if !hasNotificationsEnabled {
		config.NotificationsEnabled = true
		changed = true
		m.logInfo("配置缺少 notificationsEnabled，已按默认值迁移", "path", configPath, "enabled", true)
	}
	if !hasTrayEnabled {
		config.TrayEnabled = true
		changed = true
		m.logInfo("配置缺少 trayEnabled，已按默认值迁移", "path", configPath, "enabled", true)
	}

	// 补全缺失的配置项，兼容不同版本的配置文件
	config.Repair()
	if !reflect.DeepEqual(original, config) {
		changed = true
	}

	m.config = config
	return true, changed
}

// Save 保存配置
func (m *Manager) Save() error {
	// 保存到默认目录
	defaultConfigDir := m.GetDefaultConfigDir()
	defaultConfigPath := filepath.Join(defaultConfigDir, "config.json")

	m.logDebug("保存配置到默认目录", "path", defaultConfigPath, "source", "default")

	// 确保默认配置目录存在
	if err := os.MkdirAll(defaultConfigDir, 0755); err != nil {
		m.logError("创建默认配置目录失败", "path", defaultConfigDir, "error", err)
		return err
	}

	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		m.logError("序列化配置失败", "error", err)
		return err
	}

	// 检查配置是否实际发生变化
	if existingData, err := os.ReadFile(defaultConfigPath); err == nil {
		if string(existingData) == string(data) {
			m.config.ConfigPath = defaultConfigPath
			return nil
		}
	}

	if err := os.WriteFile(defaultConfigPath, data, 0644); err != nil {
		m.logError("保存配置失败", "path", defaultConfigPath, "error", err)
		return err
	}

	m.config.ConfigPath = defaultConfigPath
	m.logDebug("配置保存成功", "path", defaultConfigPath, "source", "default")
	return nil
}

// GetDefaultConfigDir 获取默认配置目录
func (m *Manager) GetDefaultConfigDir() string {
	programData := os.Getenv("PROGRAMDATA")
	if programData == "" {
		m.logError("PROGRAMDATA 环境变量未设置，回落到安装目录", "source", "PROGRAMDATA", "path", m.installDir)
		return filepath.Join(m.installDir, "config")
	}
	return filepath.Join(programData, "BS2PRO-Controller")
}

// GetStateDir 获取运行时状态目录
func (m *Manager) GetStateDir() string {
	return filepath.Join(m.GetDefaultConfigDir(), "state")
}

// Get 获取当前配置
func (m *Manager) Get() types.AppConfig {
	return m.config
}

// Update 更新配置并保存
func (m *Manager) Update(config types.AppConfig) error {
	config.Repair()
	m.config = config
	return m.Save()
}

// 日志辅助方法
func (m *Manager) logInfo(msg any, args ...any) {
	if m.logger != nil {
		m.logger.Info(msg, args...)
	}
}

func (m *Manager) logError(msg any, args ...any) {
	if m.logger != nil {
		m.logger.Error(msg, args...)
	}
}

func (m *Manager) logDebug(msg any, args ...any) {
	if m.logger != nil {
		m.logger.Debug(msg, args...)
	}
}

// GetInstallDir 获取安装目录
func GetInstallDir() string {
	exePath, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exePath)
}

// GetLogDir 获取日志目录，统一存放到ProgramData\BS2PRO-Controller\logs
func GetLogDir() string {
	programData := os.Getenv("PROGRAMDATA")
	if programData != "" {
		return filepath.Join(programData, "BS2PRO-Controller", "logs")
	}
	// 回落：ProgramData环境变量不存在时用硬编码路径
	return `C:\ProgramData\BS2PRO-Controller\logs`
}
