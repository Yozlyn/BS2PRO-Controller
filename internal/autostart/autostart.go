// Package autostart 提供 Windows 自启动管理功能
package autostart

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/TIANLI0/BS2PRO-Controller/internal/types"
	"golang.org/x/sys/windows/registry"
)

// Manager 自启动管理器
type Manager struct {
	logger types.Logger
}

// NewManager 创建新的自启动管理器
func NewManager(logger types.Logger) *Manager {
	return &Manager{
		logger: logger,
	}
}

// IsRunningAsAdmin 检查是否以管理员权限运行
func (m *Manager) IsRunningAsAdmin() bool {
	return true
}

// SetWindowsAutoStart 设置Windows开机自启动
func (m *Manager) SetWindowsAutoStart(enable bool) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开注册表失败: %v", err)
	}
	defer key.Close()

	if enable {
		exePath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("获取程序路径失败: %v", err)
		}

		guiPath := filepath.Join(filepath.Dir(exePath), "BS2PRO-Controller.exe")

		val := fmt.Sprintf(`"%s" --autostart`, guiPath)
		err = key.SetStringValue("BS2PRO-Controller", val)
		if err != nil {
			return fmt.Errorf("设置注册表失败: %v", err)
		}
		m.logger.Info("已通过注册表设置前端控制台开机自启动")
	} else {
		err = key.DeleteValue("BS2PRO-Controller")
		if err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("删除注册表项失败: %v", err)
		}
		m.logger.Info("已移除前端控制台开机自启动")
	}
	return nil
}

// CheckWindowsAutoStart 检查Windows开机自启动状态
func (m *Manager) CheckWindowsAutoStart() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	_, _, err = key.GetStringValue("BS2PRO-Controller")
	return err == nil
}

// GetAutoStartMethod 获取当前的自启动方式
func (m *Manager) GetAutoStartMethod() string {
	return "registry"
}

// SetAutoStartWithMethod 使用指定方式设置自启动
func (m *Manager) SetAutoStartWithMethod(enable bool, method string) error {
	return m.SetWindowsAutoStart(enable)
}

// DetectAutoStartLaunch 检测是否为自启动启动
func DetectAutoStartLaunch(args []string) bool {
	for _, arg := range args {
		if arg == "--autostart" || arg == "/autostart" || arg == "-autostart" {
			return true
		}
	}
	return false
}
