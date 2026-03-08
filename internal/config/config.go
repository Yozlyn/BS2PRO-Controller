// Package config 提供配置管理功能
package config

import (
	"encoding/json"
	"os"
	"path/filepath"

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

	m.logInfo("尝试从默认目录加载配置: %s", defaultConfigPath)

	// 先尝试从默认目录加载
	if m.tryLoadFromPath(defaultConfigPath) {
		m.config.ConfigPath = defaultConfigPath
		m.logInfo("从默认目录加载配置成功: %s", defaultConfigPath)
		return m.config
	}

	m.logInfo("从默认目录加载配置失败，尝试从安装目录加载: %s", installConfigPath)

	// 默认目录失败，尝试从安装目录加载
	if m.tryLoadFromPath(installConfigPath) {
		m.config.ConfigPath = installConfigPath
		m.logInfo("从安装目录加载配置成功: %s", installConfigPath)
		return m.config
	}

	m.logError("所有配置目录加载失败，使用默认配置")

	m.config = types.GetDefaultConfig(isAutoStart)
	m.config.ConfigPath = defaultConfigPath
	if err := m.Save(); err != nil {
		m.logError("保存默认配置失败: %v", err)
	}

	return m.config
}

// tryLoadFromPath 尝试从指定路径加载配置
func (m *Manager) tryLoadFromPath(configPath string) bool {
	if _, err := os.Stat(configPath); err != nil {
		m.logDebug("配置文件不存在: %s", configPath)
		return false
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		m.logError("读取配置文件失败 %s: %v", configPath, err)
		return false
	}

	var config types.AppConfig
	if err := json.Unmarshal(data, &config); err != nil {
		m.logError("解析配置文件失败 %s: %v", configPath, err)
		return false
	}

	m.config = config
	return true
}

// Save 保存配置
func (m *Manager) Save() error {
	// 保存到默认目录
	defaultConfigDir := m.GetDefaultConfigDir()
	defaultConfigPath := filepath.Join(defaultConfigDir, "config.json")

	m.logDebug("保存配置到默认目录: %s", defaultConfigPath)

	// 确保默认配置目录存在
	if err := os.MkdirAll(defaultConfigDir, 0755); err != nil {
		m.logError("创建默认配置目录失败: %v", err)
		return err
	}

	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		m.logError("序列化配置失败: %v", err)
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
		m.logError("保存配置失败: %v", err)
		return err
	}

	m.config.ConfigPath = defaultConfigPath
	m.logInfo("配置保存成功: %s", defaultConfigPath)
	return nil
}

// GetDefaultConfigDir 获取默认配置目录
func (m *Manager) GetDefaultConfigDir() string {
	programData := os.Getenv("PROGRAMDATA")
	if programData == "" {
		m.logError("PROGRAMDATA 环境变量未设置，回落到安装目录")
		return filepath.Join(m.installDir, "config")
	}
	return filepath.Join(programData, "BS2PRO-Controller")
}

// Get 获取当前配置
func (m *Manager) Get() types.AppConfig {
	return m.config
}

// Update 更新配置并保存
func (m *Manager) Update(config types.AppConfig) error {
	m.config = config
	return m.Save()
}

// 日志辅助方法
func (m *Manager) logInfo(format string, v ...any) {
	if m.logger != nil {
		m.logger.Info(format, v...)
	}
}

func (m *Manager) logError(format string, v ...any) {
	if m.logger != nil {
		m.logger.Error(format, v...)
	}
}

func (m *Manager) logDebug(format string, v ...any) {
	if m.logger != nil {
		m.logger.Debug(format, v...)
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
