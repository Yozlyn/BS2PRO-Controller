//go:build windows

package notification

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/TIANLI0/BS2PRO-Controller/internal/config"
	toast "github.com/go-toast/toast"
)

const defaultAppID = "BS2PRO-Controller"

func Send(appID, title, message string) error {
	if appID == "" {
		appID = defaultAppID
	}
	n := toast.Notification{
		AppID:   appID,
		Title:   title,
		Message: message,
		Icon:    resolveToastIconPath(),
	}
	if err := n.Push(); err != nil {
		return fmt.Errorf("发送Windows通知失败: %w", err)
	}
	return nil
}

func resolveToastIconPath() string {
	installDir := config.GetInstallDir()
	candidates := []string{
		filepath.Join(installDir, "appicon.png"),
		filepath.Join(installDir, "build", "appicon.png"),
		filepath.Join(installDir, "icon.png"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}
