// Package notification 提供 Windows 系统 Toast 通知功能
package notification

import (
	"fmt"
	"os/exec"
	"strings"
)

const defaultAppID = "BS2PRO-Controller"
const defaultAppName = "BS2PRO Controller"

// Send 发送 Windows Toast 系统通知。
//
//   - appID:   通知来源标识，留空时使用默认值 "BS2PRO-Controller"
//   - title:   通知标题
//   - message: 通知正文
func Send(appID, title, message string) error {
	if appID == "" {
		appID = defaultAppID
	}

	// 对单引号做转义，防止 PowerShell 脚本注入
	safeAppID := strings.ReplaceAll(appID, "'", "''")
	safeTitle := strings.ReplaceAll(title, "'", "''")
	safeMessage := strings.ReplaceAll(message, "'", "''")
	script := fmt.Sprintf(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType=WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType=WindowsRuntime] | Out-Null
$appId    = '%s'
$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent(
                [Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$template.GetElementsByTagName('text')[0].AppendChild($template.CreateTextNode('%s')) | Out-Null
$template.GetElementsByTagName('text')[1].AppendChild($template.CreateTextNode('%s')) | Out-Null
$toast    = [Windows.UI.Notifications.ToastNotification]::new($template)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($appId).Show($toast)
`, safeAppID, safeTitle, safeMessage)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
	return cmd.Run()
}
