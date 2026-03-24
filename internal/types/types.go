// Package types 定义了 BS2PRO 控制器应用中使用的所有共享类型
package types

import (
	"sort"
	"strings"
)

// FanCurvePoint 风扇曲线点
type FanCurvePoint struct {
	Temperature int `json:"temperature"` // 温度 °C
	RPM         int `json:"rpm"`         // 转速 RPM
	Offset      int `json:"offset"`      // 转速偏移 RPM (100的整数倍)
}

// FanData 风扇数据结构
type FanData struct {
	ReportID     uint8  `json:"reportId"`
	MagicSync    uint16 `json:"magicSync"`
	Command      uint8  `json:"command"`
	Status       uint8  `json:"status"`
	GearSettings uint8  `json:"gearSettings"`
	CurrentMode  uint8  `json:"currentMode"`
	Reserved1    uint8  `json:"reserved1"`
	CurrentRPM   uint16 `json:"currentRpm"`
	TargetRPM    uint16 `json:"targetRpm"`
	MaxGear      string `json:"maxGear"`
	SetGear      string `json:"setGear"`
	WorkMode     string `json:"workMode"`
}

// GearCommand 挡位命令结构
type GearCommand struct {
	Name    string `json:"name"`    // 挡位名称
	Command []byte `json:"command"` // 命令字节
	RPM     int    `json:"rpm"`     // 对应转速
}

// TemperatureData 温度数据
type TemperatureData struct {
	CPUTemp     int    `json:"cpuTemp"`       // CPU温度
	GPUTemp     int    `json:"gpuTemp"`       // GPU温度
	MaxTemp     int    `json:"maxTemp"`       // 最高温度
	UpdateTime  int64  `json:"updateTime"`    // 更新时间戳
	BridgeOk    bool   `json:"bridgeOk"`      // 桥接程序是否正常
	BridgeMsg   string `json:"bridgeMessage"` // 桥接故障提示
	AutoOffset  int    `json:"autoOffset"`    // 自动偏移量 (由 fanoffset 控制器计算)
	EngineState string `json:"engineState"`   // 当前温度区间引擎状态（补偿中/回收中/稳定）
}

// BridgeTemperatureData 桥接程序返回的温度数据
type BridgeTemperatureData struct {
	CpuTemp    int    `json:"cpuTemp"`
	GpuTemp    int    `json:"gpuTemp"`
	MaxTemp    int    `json:"maxTemp"`
	UpdateTime int64  `json:"updateTime"`
	Success    bool   `json:"success"`
	Error      string `json:"error"`
}

// RGBColorConfig RGB颜色配置
type RGBColorConfig struct {
	R int `json:"r"`
	G int `json:"g"`
	B int `json:"b"`
}

// RGBConfig RGB灯效配置
type RGBConfig struct {
	Mode       string           `json:"mode"`
	Colors     []RGBColorConfig `json:"colors"`
	Speed      string           `json:"speed"`
	Brightness int              `json:"brightness"`
}

// ProcessFanRule 进程联动规则
type ProcessFanRule struct {
	ProcessName string `json:"processName"` // 进程名，如 game.exe
	ProfilePath string `json:"profilePath"` // 风扇曲线文件路径
	Enabled     bool   `json:"enabled"`     // 规则开关
}

// AppConfig 应用配置
type AppConfig struct {
	AutoControl             bool             `json:"autoControl"`             // 智能变频开关
	FanCurve                []FanCurvePoint  `json:"fanCurve"`                // 风扇曲线
	GearLight               bool             `json:"gearLight"`               // 挡位灯
	NotificationsEnabled    bool             `json:"notificationsEnabled"`    // 系统通知开关
	Hotkeys                 *HotkeyConfig    `json:"hotkeys"`                 // 快捷键配置
	PowerOnStart            bool             `json:"powerOnStart"`            // 通电自启动
	WindowsAutoStart        bool             `json:"windowsAutoStart"`        // Windows开机自启动
	MonitorAutoStart        bool             `json:"monitorAutoStart"`        // Monitor开机自启动
	SmartStartStop          string           `json:"smartStartStop"`          // 智能启停
	Brightness              int              `json:"brightness"`              // 亮度
	TempUpdateRate          int              `json:"tempUpdateRate"`          // 温度更新频率(秒)
	TempSampleCount         int              `json:"tempSampleCount"`         // 温度采样次数(用于平均)
	ConfigPath              string           `json:"configPath"`              // 配置文件路径
	ManualGear              string           `json:"manualGear"`              // 手动挡位设置
	ManualLevel             string           `json:"manualLevel"`             // 手动挡位级别(低中高)
	DebugMode               bool             `json:"debugMode"`               // 调试模式
	GuiMonitoring           bool             `json:"guiMonitoring"`           // GUI监控开关
	CustomSpeedEnabled      bool             `json:"customSpeedEnabled"`      // 自定义转速开关
	CustomSpeedRPM          int              `json:"customSpeedRPM"`          // 自定义转速值(无上下限)
	IgnoreDeviceOnReconnect bool             `json:"ignoreDeviceOnReconnect"` // 断连后忽略设备状态(保持APP配置)
	FanCurveOffsetEnabled   bool             `json:"fanCurveOffsetEnabled"`   // 风扇曲线偏移开关
	ProcessSwitchEnabled    bool             `json:"processSwitchEnabled"`    // 进程联动风扇配置
	ProcessSwitchInterval   int              `json:"processSwitchInterval"`   // 进程扫描周期(秒)
	ProcessSwitchRules      []ProcessFanRule `json:"processSwitchRules"`      // 进程联动规则
	RGBConfig               *RGBConfig       `json:"rgbConfig"`               // RGB灯效配置
	HotkeyConflicts         []HotkeyConflict `json:"hotkeyConflicts,omitempty"`
}

type HotkeyConfig struct {
	Enabled bool            `json:"enabled"`
	Global  []HotkeyBinding `json:"global"`
	InApp   []HotkeyBinding `json:"inApp"`
}

type HotkeyBinding struct {
	Action      string `json:"action"`
	Accelerator string `json:"accelerator"`
	Scope       string `json:"scope"`
	Enabled     bool   `json:"enabled"`
	Editable    bool   `json:"editable"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

type HotkeyConflict struct {
	Accelerator string   `json:"accelerator"`
	Scopes      []string `json:"scopes"`
	Actions     []string `json:"actions"`
	Source      string   `json:"source,omitempty"`
	Message     string   `json:"message,omitempty"`
}

// Logger 日志记录器接口
type Logger interface {
	Info(format string, v ...any)
	Error(format string, v ...any)
	Warn(format string, v ...any)
	Debug(format string, v ...any)
	Close()
	CleanOldLogs()
	SetDebugMode(enabled bool)
	GetLogDir() string
}

// GearCommands 预设挡位命令
var GearCommands = map[string][]GearCommand{
	"静音": {
		{"1挡低", []byte{0x5a, 0xa5, 0x26, 0x05, 0x00, 0x14, 0x05, 0x44, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 1300},
		{"1挡中", []byte{0x5a, 0xa5, 0x26, 0x05, 0x00, 0xa4, 0x06, 0xd5, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 1700},
		{"1挡高", []byte{0x5a, 0xa5, 0x26, 0x05, 0x00, 0x6c, 0x07, 0x9e, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 1900},
	},
	"标准": {
		{"2挡低", []byte{0x5a, 0xa5, 0x26, 0x05, 0x01, 0x34, 0x08, 0x68, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 2100},
		{"2挡中", []byte{0x5a, 0xa5, 0x26, 0x05, 0x01, 0x60, 0x09, 0x95, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 2310},
		{"2挡高", []byte{0x5a, 0xa5, 0x26, 0x05, 0x01, 0x8c, 0x0a, 0xc2, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 2760},
	},
	"强劲": {
		{"3挡低", []byte{0x5a, 0xa5, 0x26, 0x05, 0x02, 0xf0, 0x0a, 0x27, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 2800},
		{"3挡中", []byte{0x5a, 0xa5, 0x26, 0x05, 0x02, 0xb8, 0x0b, 0xf0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 3000},
		{"3挡高", []byte{0x5a, 0xa5, 0x26, 0x05, 0x02, 0xe4, 0x0c, 0x1d, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 3300},
	},
	"超频": {
		{"4挡低", []byte{0x5a, 0xa5, 0x26, 0x05, 0x03, 0xac, 0x0d, 0xe7, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 3500},
		{"4挡中", []byte{0x5a, 0xa5, 0x26, 0x05, 0x03, 0x74, 0x0e, 0xb0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 3700},
		{"4挡高", []byte{0x5a, 0xa5, 0x26, 0x05, 0x03, 0xa0, 0x0f, 0xdd, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 4000},
	},
}

// GetDefaultFanCurve 获取默认风扇曲线
func GetDefaultFanCurve() []FanCurvePoint {
	return []FanCurvePoint{
		{Temperature: 30, RPM: 500, Offset: 0},
		{Temperature: 35, RPM: 1200, Offset: 0},
		{Temperature: 40, RPM: 1400, Offset: 0},
		{Temperature: 45, RPM: 1600, Offset: 0},
		{Temperature: 50, RPM: 1800, Offset: 100},
		{Temperature: 55, RPM: 2000, Offset: 100},
		{Temperature: 60, RPM: 2300, Offset: 100},
		{Temperature: 65, RPM: 2600, Offset: 200},
		{Temperature: 70, RPM: 2900, Offset: 200},
		{Temperature: 75, RPM: 3200, Offset: 200},
		{Temperature: 80, RPM: 3500, Offset: 300},
		{Temperature: 85, RPM: 3800, Offset: 200},
		{Temperature: 90, RPM: 4000, Offset: 0},
		{Temperature: 95, RPM: 4000, Offset: 0},
		{Temperature: 100, RPM: 4000, Offset: 0},
	}
}

// GetDefaultConfig 获取默认配置
func GetDefaultConfig(isAutoStart bool) AppConfig {
	return AppConfig{
		AutoControl:             false,
		FanCurve:                GetDefaultFanCurve(),
		GearLight:               true,
		NotificationsEnabled:    true,
		Hotkeys:                 GetDefaultHotkeyConfig(),
		PowerOnStart:            false,
		WindowsAutoStart:        false,
		MonitorAutoStart:        true,
		SmartStartStop:          "off",
		Brightness:              100,
		TempUpdateRate:          2,
		TempSampleCount:         1,
		ConfigPath:              "",
		ManualGear:              "标准",
		ManualLevel:             "中",
		DebugMode:               false,
		GuiMonitoring:           true,
		CustomSpeedEnabled:      false,
		CustomSpeedRPM:          2000,
		IgnoreDeviceOnReconnect: true, // 默认开启，防止断连后误判用户手动切换
		FanCurveOffsetEnabled:   false,
		ProcessSwitchEnabled:    false,
		ProcessSwitchInterval:   3,
		ProcessSwitchRules:      []ProcessFanRule{},
		RGBConfig: &RGBConfig{
			Mode:       "smart",
			Colors:     []RGBColorConfig{{R: 0, G: 0, B: 255}, {R: 255, G: 0, B: 0}, {R: 0, G: 255, B: 0}},
			Speed:      "medium",
			Brightness: 100,
		},
	}
}

func GetDefaultHotkeyConfig() *HotkeyConfig {
	return &HotkeyConfig{
		Enabled: true,
		Global: []HotkeyBinding{
			{Action: "show-main-window", Accelerator: "Ctrl+Alt+B", Scope: "global", Enabled: true, Editable: true, Description: "显示或隐藏主窗口", Category: "窗口"},
			{Action: "toggle-auto-control", Accelerator: "Ctrl+Alt+A", Scope: "global", Enabled: true, Editable: true, Description: "切换智能变频", Category: "设备"},
			{Action: "toggle-process-switch", Accelerator: "Ctrl+Alt+P", Scope: "global", Enabled: true, Editable: true, Description: "切换进程联动", Category: "设备"},
			{Action: "cycle-rgb-mode", Accelerator: "Ctrl+Alt+R", Scope: "global", Enabled: true, Editable: true, Description: "切换 RGB 灯光模式", Category: "RGB"},
		},
		InApp: []HotkeyBinding{
			{Action: "save-context", Accelerator: "Ctrl+S", Scope: "app", Enabled: true, Editable: true, Description: "保存当前页面", Category: "通用"},
			{Action: "escape-local-interaction", Accelerator: "Escape", Scope: "app", Enabled: true, Editable: true, Description: "取消页面交互", Category: "通用"},
			{Action: "toggle-sidebar", Accelerator: "Ctrl+B", Scope: "app", Enabled: true, Editable: true, Description: "切换侧边栏", Category: "通用"},
			{Action: "navigate-dashboard", Accelerator: "Alt+1", Scope: "app", Enabled: true, Editable: true, Description: "切换到设备概览", Category: "导航"},
			{Action: "navigate-fan-curve", Accelerator: "Alt+2", Scope: "app", Enabled: true, Editable: true, Description: "切换到风扇曲线", Category: "导航"},
			{Action: "navigate-device-params", Accelerator: "Alt+3", Scope: "app", Enabled: true, Editable: true, Description: "切换到设备参数", Category: "导航"},
			{Action: "navigate-rgb-light", Accelerator: "Alt+4", Scope: "app", Enabled: true, Editable: true, Description: "切换到 RGB 灯效", Category: "导航"},
			{Action: "navigate-process-switch", Accelerator: "Alt+5", Scope: "app", Enabled: true, Editable: true, Description: "切换到进程联动", Category: "导航"},
			{Action: "navigate-system-settings", Accelerator: "Alt+6", Scope: "app", Enabled: true, Editable: true, Description: "切换到系统设置", Category: "导航"},
			{Action: "navigate-about", Accelerator: "Alt+7", Scope: "app", Enabled: true, Editable: true, Description: "切换到关于软件", Category: "导航"},
		},
	}
}

func NormalizeAccelerator(accelerator string) string {
	parts := strings.Split(strings.TrimSpace(accelerator), "+")
	mods := map[string]bool{}
	key := ""
	for _, part := range parts {
		token := strings.TrimSpace(strings.ToLower(part))
		switch token {
		case "control", "ctrl":
			mods["Ctrl"] = true
		case "alternate", "alt":
			mods["Alt"] = true
		case "shift":
			mods["Shift"] = true
		case "meta", "cmd", "win", "super":
			mods["Meta"] = true
		default:
			if token == "escape" {
				key = "Escape"
			} else {
				key = strings.ToUpper(token)
			}
		}
	}
	ordered := make([]string, 0, 5)
	for _, mod := range []string{"Ctrl", "Alt", "Shift", "Meta"} {
		if mods[mod] {
			ordered = append(ordered, mod)
		}
	}
	if key != "" {
		ordered = append(ordered, key)
	}
	return strings.Join(ordered, "+")
}

func DetectHotkeyConflicts(cfg *HotkeyConfig) []HotkeyConflict {
	if cfg == nil {
		return nil
	}
	type item struct{ scope, action string }
	index := map[string][]item{}
	for _, binding := range append(append([]HotkeyBinding{}, cfg.Global...), cfg.InApp...) {
		if !binding.Enabled {
			continue
		}
		accelerator := NormalizeAccelerator(binding.Accelerator)
		if accelerator == "" {
			continue
		}
		index[accelerator] = append(index[accelerator], item{scope: binding.Scope, action: binding.Action})
	}
	conflicts := make([]HotkeyConflict, 0)
	for accelerator, items := range index {
		groups := map[string]map[string]bool{}
		for _, item := range items {
			if groups[item.scope] == nil {
				groups[item.scope] = map[string]bool{}
			}
			groups[item.scope][item.action] = true
		}
		for scope, actions := range groups {
			if len(actions) < 2 {
				continue
			}
			list := make([]string, 0, len(actions))
			for action := range actions {
				list = append(list, action)
			}
			sort.Strings(list)
			conflicts = append(conflicts, HotkeyConflict{Accelerator: accelerator, Scopes: []string{scope}, Actions: list})
		}
	}
	sort.Slice(conflicts, func(i, j int) bool { return conflicts[i].Accelerator < conflicts[j].Accelerator })
	return conflicts
}

// Repair 检查并修复配置中缺失的值
func (c *AppConfig) Repair() {
	defaultCfg := GetDefaultConfig(c.WindowsAutoStart)

	if len(c.FanCurve) == 0 {
		c.FanCurve = defaultCfg.FanCurve
	} else {
		// 迁移：若曲线最高温度点 < 100°C，自动追加 100°C 点
		last := c.FanCurve[len(c.FanCurve)-1]
		if last.Temperature < 100 {
			c.FanCurve = append(c.FanCurve, FanCurvePoint{
				Temperature: 100,
				RPM:         last.RPM,
				Offset:      0,
			})
		}
	}
	if c.SmartStartStop == "" {
		c.SmartStartStop = defaultCfg.SmartStartStop
	}
	if c.TempUpdateRate <= 0 {
		c.TempUpdateRate = defaultCfg.TempUpdateRate
	}
	if c.TempSampleCount <= 0 {
		c.TempSampleCount = defaultCfg.TempSampleCount
	}
	if c.ManualGear == "" {
		c.ManualGear = defaultCfg.ManualGear
	}
	if c.ManualLevel == "" {
		c.ManualLevel = defaultCfg.ManualLevel
	}
	if c.CustomSpeedRPM <= 0 {
		c.CustomSpeedRPM = defaultCfg.CustomSpeedRPM
	}
	if c.ProcessSwitchInterval <= 0 {
		c.ProcessSwitchInterval = defaultCfg.ProcessSwitchInterval
	}
	if c.ProcessSwitchRules == nil {
		c.ProcessSwitchRules = []ProcessFanRule{}
	}

	if c.RGBConfig == nil {
		c.RGBConfig = defaultCfg.RGBConfig
	} else {
		if c.RGBConfig.Mode == "" {
			c.RGBConfig.Mode = defaultCfg.RGBConfig.Mode
		}
		if len(c.RGBConfig.Colors) == 0 {
			c.RGBConfig.Colors = defaultCfg.RGBConfig.Colors
		}
		if c.RGBConfig.Speed == "" {
			c.RGBConfig.Speed = defaultCfg.RGBConfig.Speed
		}
		// 不对 Brightness 为 0 强行覆盖，因为可能用户就想设0亮度
	}
	if c.Hotkeys == nil {
		c.Hotkeys = defaultCfg.Hotkeys
	} else {
		c.Hotkeys.Global = repairHotkeyBindings(c.Hotkeys.Global, defaultCfg.Hotkeys.Global)
		c.Hotkeys.InApp = repairHotkeyBindings(c.Hotkeys.InApp, defaultCfg.Hotkeys.InApp)
	}
}

func repairHotkeyBindings(current []HotkeyBinding, defaults []HotkeyBinding) []HotkeyBinding {
	index := map[string]HotkeyBinding{}
	orderedExtras := make([]HotkeyBinding, 0)
	defaultActions := map[string]bool{}
	for _, item := range defaults {
		defaultActions[item.Action] = true
	}
	for _, item := range current {
		index[item.Action] = item
		if !defaultActions[item.Action] {
			orderedExtras = append(orderedExtras, item)
		}
	}
	result := make([]HotkeyBinding, 0, len(defaults))
	for _, item := range defaults {
		existing, ok := index[item.Action]
		if !ok {
			result = append(result, item)
			continue
		}
		if existing.Accelerator == "" {
			existing.Accelerator = item.Accelerator
		}
		existing.Accelerator = NormalizeAccelerator(existing.Accelerator)
		if existing.Scope == "" {
			existing.Scope = item.Scope
		}
		existing.Editable = item.Editable
		existing.Category = item.Category
		existing.Description = item.Description
		result = append(result, existing)
	}
	for _, extra := range orderedExtras {
		extra.Accelerator = NormalizeAccelerator(extra.Accelerator)
		if extra.Scope == "" {
			extra.Scope = "global"
		}
		result = append(result, extra)
	}
	return result
}
