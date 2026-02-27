// Package device 提供 HID 设备通信功能
package device

import (
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/TIANLI0/BS2PRO-Controller/internal/rgb"
	"github.com/TIANLI0/BS2PRO-Controller/internal/types"
	"github.com/sstallion/go-hid"
)

const (
	// VendorID 设备厂商ID
	VendorID = 0x37D7
	// ProductID1 产品ID 1 BS2PRO
	ProductID1 = 0x1002
	// ProductID2 产品ID 2 BS2
	ProductID2 = 0x1001
)

// Manager HID 设备管理器
type Manager struct {
	device         *hid.Device
	isConnected    bool
	productID      uint16 // 当前连接的产品ID
	mutex          sync.RWMutex
	deviceOpMutex  sync.Mutex // 设备操作互斥锁，确保同一时间只有一个读/写操作
	logger         types.Logger
	currentFanData *types.FanData

	// RGB 控制器与ACK通道
	rgbCtrl    *rgb.Controller
	rgbAckChan chan []byte

	// 回调函数
	onFanDataUpdate func(data *types.FanData)
	onDisconnect    func()
}

// NewManager 创建新的设备管理器
func NewManager(logger types.Logger) *Manager {
	m := &Manager{
		logger:     logger,
		rgbAckChan: make(chan []byte, 100),
	}
	// 注入自己作为 RGB 的底层传输通道 (实现 rgb.Transport 接口)
	m.rgbCtrl = rgb.NewController(m)
	return m
}

// RGB 获取 RGB 控制器实例
func (m *Manager) RGB() *rgb.Controller {
	return m.rgbCtrl
}

// SetCallbacks 设置回调函数
func (m *Manager) SetCallbacks(onFanDataUpdate func(data *types.FanData), onDisconnect func()) {
	m.onFanDataUpdate = onFanDataUpdate
	m.onDisconnect = onDisconnect
}

// Init 初始化 HID 库
func (m *Manager) Init() error {
	return hid.Init()
}

// Exit 清理 HID 库
func (m *Manager) Exit() error {
	return hid.Exit()
}

// Connect 连接 HID 设备
func (m *Manager) Connect() (bool, map[string]string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.isConnected {
		return true, nil
	}

	productIDs := []uint16{ProductID1, ProductID2}
	var device *hid.Device
	var err error

	var connectedProductID uint16
	for _, productID := range productIDs {
		m.logInfo("正在连接设备 - 厂商ID: 0x%04X, 产品ID: 0x%04X", VendorID, productID)

		device, err = hid.OpenFirst(VendorID, productID)
		if err == nil {
			m.logInfo("成功连接到产品ID: 0x%04X", productID)
			connectedProductID = productID
			break
		} else {
			m.logError("产品ID 0x%04X 连接失败: %v", productID, err)
		}
	}

	if err != nil {
		m.logError("所有设备连接尝试都失败")
		return false, nil
	}

	m.device = device
	m.isConnected = true
	m.productID = connectedProductID

	modelName := "BS2PRO"
	if connectedProductID == ProductID2 {
		modelName = "BS2"
	}

	// 获取设备信息
	deviceInfo, err := device.GetDeviceInfo()
	var info map[string]string
	if err == nil {
		m.logInfo("设备连接成功: %s %s (型号: %s)", deviceInfo.MfrStr, deviceInfo.ProductStr, modelName)
		info = map[string]string{
			"manufacturer": deviceInfo.MfrStr,
			"product":      deviceInfo.ProductStr,
			"serial":       deviceInfo.SerialNbr,
			"model":        modelName,
			"productId":    fmt.Sprintf("0x%04X", connectedProductID),
		}
	} else {
		m.logError("设备连接成功,但获取设备信息失败: %v", err)
		info = map[string]string{
			"manufacturer": "Unknown",
			"product":      modelName,
			"serial":       "Unknown",
			"model":        modelName,
			"productId":    fmt.Sprintf("0x%04X", connectedProductID),
		}
	}

	// 清空可能残留的 ACK
	for len(m.rgbAckChan) > 0 {
		<-m.rgbAckChan
	}

	// 通知RGB控制器开始工作
	m.rgbCtrl.Start()

	// 开始监控设备数据
	go m.monitorDeviceData()

	return true, info
}

// Disconnect 断开设备连接
func (m *Manager) Disconnect() {
	m.mutex.Lock()

	if !m.isConnected {
		m.mutex.Unlock()
		return
	}

	// 停止RGB控制器
	m.rgbCtrl.Stop()

	// 关闭设备
	if m.device != nil {
		m.device.Close()
		m.device = nil
	}

	m.isConnected = false
	m.mutex.Unlock()
	m.logInfo("设备连接已断开")
}

// IsConnected 检查设备是否已连接
func (m *Manager) IsConnected() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.isConnected
}

// GetCurrentFanData 获取当前风扇数据
func (m *Manager) GetCurrentFanData() *types.FanData {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.currentFanData
}

// monitorDeviceData 监控设备数据
func (m *Manager) monitorDeviceData() {
	m.mutex.RLock()
	if !m.isConnected || m.device == nil {
		m.mutex.RUnlock()
		return
	}
	m.mutex.RUnlock()

	buffer := make([]byte, 64)
	consecutiveErrors := 0
	const maxConsecutiveErrors = 5

	for {
		m.mutex.RLock()
		connected := m.isConnected
		device := m.device
		m.mutex.RUnlock()

		if !connected || device == nil {
			break
		}

		m.deviceOpMutex.Lock()
		n, err := device.ReadWithTimeout(buffer, 100*time.Millisecond)
		m.deviceOpMutex.Unlock()

		if err != nil {
			if err == hid.ErrTimeout {
				consecutiveErrors = 0
				continue
			}

			consecutiveErrors++
			if consecutiveErrors >= maxConsecutiveErrors {
				m.logError("连续读取失败，设备断开: %v", err)
				break
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}

		consecutiveErrors = 0

		if n > 0 {
			// 将数据抄送给RGB拦截器
			m.extractRGBACK(buffer, n)

			// 解析风扇数据
			fanData := m.parseFanData(buffer, n)
			if fanData != nil {
				m.mutex.Lock()
				m.currentFanData = fanData
				m.mutex.Unlock()

				if m.onFanDataUpdate != nil {
					m.onFanDataUpdate(fanData)
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	m.handleDeviceDisconnected()
}

// extractRGBACK 拦截提取硬件发回的RGB确认包
func (m *Manager) extractRGBACK(buf []byte, n int) {
	if n < 5 {
		return
	}
	for i := 0; i < n-4; i++ {
		if buf[i] == 0x5A && buf[i+1] == 0xA5 {
			length := int(buf[i+3])
			totalPacketLen := length + 3

			if totalPacketLen > 0 && i+totalPacketLen <= n {
				packet := make([]byte, totalPacketLen)
				copy(packet, buf[i:i+totalPacketLen])

				// 避开风扇数据帧(0xEF)，将ACK推入通道
				if packet[2] != 0xEF {
					select {
					case m.rgbAckChan <- packet:
					default:
					}
				}
				i += totalPacketLen - 1
			}
		}
	}
}

// ----- 实现 rgb.Transport 接口方法 -----

// WritePacket 将组装好的 RGB 数据包加上 HID Report ID 并发送，不等待确认
func (m *Manager) WritePacket(packet []byte) error {
	m.mutex.RLock()
	dev := m.device
	m.mutex.RUnlock()

	if dev == nil {
		return fmt.Errorf("设备未连接")
	}

	// 封装成32字节 HID 帧，头部带 0x02
	buf := make([]byte, 32)
	buf[0] = 0x02
	copy(buf[1:], packet)

	m.deviceOpMutex.Lock()
	_, err := dev.Write(buf)
	m.deviceOpMutex.Unlock()
	return err
}

// WritePacketAndWaitACK 发送数据并同步等待特定指令的 ACK，超时返回 false
func (m *Manager) WritePacketAndWaitACK(cmdID byte, packet []byte, timeout time.Duration) bool {
	// 发送前清空通道内陈旧的ACK
	for len(m.rgbAckChan) > 0 {
		<-m.rgbAckChan
	}

	// 写入数据
	if err := m.WritePacket(packet); err != nil {
		return false
	}

	// 异步等待ACK
	go func() {
		startTime := time.Now()
		timer := time.NewTimer(timeout)
		defer timer.Stop()

		select {
		case resp := <-m.rgbAckChan:
			elapsed := time.Since(startTime)
			// 放宽检查条件：只要resp[4]==1就认为是成功ACK
			if len(resp) >= 5 && resp[4] == 1 {
				m.logDebug("ACK received for cmdID 0x%02X (got 0x%02X), delay: %v",
					cmdID, resp[2], elapsed)
				return
			} else if len(resp) >= 5 {
				m.logWarn("ACK failed for cmdID 0x%02X (resp[4]=0x%02X), got cmdID 0x%02X, delay: %v",
					cmdID, resp[4], resp[2], elapsed)
			}
		case <-timer.C:
			// ACK超时，记录warning日志
			m.logWarn("ACK timeout for cmdID 0x%02X, timeout: %v", cmdID, timeout)
			return
		}
	}()

	// 立即返回true，不等待ACK
	return true
}

// handleDeviceDisconnected 处理设备断开
func (m *Manager) handleDeviceDisconnected() {
	m.mutex.Lock()
	wasConnected := m.isConnected

	if m.device != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
				}
			}()
			m.device.Close()
		}()
		m.device = nil
	}

	m.isConnected = false
	m.mutex.Unlock()

	m.rgbCtrl.Stop()

	if wasConnected {
		m.logInfo("设备连接已断开")
		if m.onDisconnect != nil {
			m.onDisconnect()
		}
	}
}

// parseFanData 解析风扇数据
func (m *Manager) parseFanData(data []byte, length int) *types.FanData {
	if length < 11 {
		return nil
	}
	magic := binary.BigEndian.Uint16(data[1:3])
	if magic != 0x5AA5 || data[3] != 0xEF {
		return nil
	}

	fanData := &types.FanData{
		ReportID:     data[0],
		MagicSync:    magic,
		Command:      data[3],
		Status:       data[4],
		GearSettings: data[5],
		CurrentMode:  data[6],
		Reserved1:    data[7],
	}

	if length >= 10 {
		fanData.CurrentRPM = binary.LittleEndian.Uint16(data[8:10])
	}
	if length >= 12 {
		fanData.TargetRPM = binary.LittleEndian.Uint16(data[10:12])
	}

	fanData.MaxGear, fanData.SetGear = m.parseGearSettings(fanData.GearSettings)
	fanData.WorkMode = m.parseWorkMode(fanData.CurrentMode)

	return fanData
}

func (m *Manager) parseGearSettings(gearByte uint8) (maxGear, setGear string) {
	maxGearCode := (gearByte >> 4) & 0x0F
	setGearCode := gearByte & 0x0F

	maxGearMap := map[uint8]string{0x2: "标准", 0x4: "强劲", 0x6: "超频"}
	setGearMap := map[uint8]string{0x8: "静音", 0xA: "标准", 0xC: "强劲", 0xE: "超频"}

	if val, ok := maxGearMap[maxGearCode]; ok {
		maxGear = val
	} else {
		maxGear = fmt.Sprintf("未知(0x%X)", maxGearCode)
	}

	if val, ok := setGearMap[setGearCode]; ok {
		setGear = val
	} else {
		setGear = fmt.Sprintf("未知(0x%X)", setGearCode)
	}

	return
}

func (m *Manager) parseWorkMode(mode uint8) string {
	switch mode {
	case 0x04, 0x02, 0x06, 0x0A, 0x08, 0x00:
		return "挡位工作模式"
	case 0x05, 0x03, 0x07, 0x0B, 0x09, 0x01:
		return "自动模式(实时转速)"
	default:
		return fmt.Sprintf("未知模式(0x%02X)", mode)
	}
}

// validateAndGetDevice 验证转速合法性并在持锁状态下取出设备引用。
// 返回 (nil, false) 表示验证失败，调用方应直接返回 false。
func (m *Manager) validateAndGetDevice(rpm int, label string) (*hid.Device, bool) {
	m.mutex.Lock()
	if !m.isConnected || m.device == nil {
		m.mutex.Unlock()
		return nil, false
	}
	if rpm < 1000 || rpm > 4000 {
		m.mutex.Unlock()
		return nil, false
	}
	if rpm%100 != 0 {
		m.mutex.Unlock()
		m.logError("%s %d 不是100的整数倍", label, rpm)
		return nil, false
	}
	dev := m.device
	m.mutex.Unlock()
	return dev, true
}

// buildSpeedCmd 构建转速下发命令（带 Report ID 0x02 前缀，总长 23 字节）
func buildSpeedCmd(rpm int) []byte {
	speedBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(speedBytes, uint16(rpm))
	checksum := (0x5A + 0xA5 + 0x21 + 0x04 + int(speedBytes[0]) + int(speedBytes[1]) + 1) & 0xFF
	cmd := []byte{0x02, 0x5A, 0xA5, 0x21, 0x04}
	cmd = append(cmd, speedBytes...)
	cmd = append(cmd, byte(checksum))
	cmd = append(cmd, make([]byte, 23-len(cmd))...)
	return cmd
}

// SetFanSpeed 设置风扇转速（纯数据下发，不再带模式切换）
func (m *Manager) SetFanSpeed(rpm int) bool {
	dev, ok := m.validateAndGetDevice(rpm, "转速")
	if !ok {
		return false
	}
	cmd := buildSpeedCmd(rpm)
	m.deviceOpMutex.Lock()
	_, err := dev.Write(cmd)
	m.deviceOpMutex.Unlock()
	return err == nil
}

// SetCustomFanSpeed 设置自定义风扇转速（先切换至自动模式再下发转速）
func (m *Manager) SetCustomFanSpeed(rpm int) bool {
	dev, ok := m.validateAndGetDevice(rpm, "自定义转速")
	if !ok {
		return false
	}

	enterModeCmd := []byte{0x02, 0x5A, 0xA5, 0x23, 0x02, 0x25, 0x00}
	enterModeCmd = append(enterModeCmd, make([]byte, 23-len(enterModeCmd))...)
	m.deviceOpMutex.Lock()
	dev.Write(enterModeCmd)
	m.deviceOpMutex.Unlock()

	time.Sleep(50 * time.Millisecond)

	cmd := buildSpeedCmd(rpm)
	m.deviceOpMutex.Lock()
	_, err := dev.Write(cmd)
	m.deviceOpMutex.Unlock()
	return err == nil
}

// EnterAutoMode 进入自动模式
func (m *Manager) EnterAutoMode() error {
	m.mutex.Lock()
	if !m.isConnected || m.device == nil {
		m.mutex.Unlock()
		return fmt.Errorf("设备未连接")
	}
	dev := m.device
	m.mutex.Unlock()

	enterModeCmd := []byte{0x02, 0x5A, 0xA5, 0x23, 0x02, 0x25, 0x00}
	enterModeCmd = append(enterModeCmd, make([]byte, 23-len(enterModeCmd))...)
	m.deviceOpMutex.Lock()
	_, err := dev.Write(enterModeCmd)
	m.deviceOpMutex.Unlock()
	return err
}

func (m *Manager) SetManualGear(gear, level string) bool {
	m.mutex.Lock()
	if !m.isConnected || m.device == nil {
		m.mutex.Unlock()
		return false
	}
	dev := m.device
	m.mutex.Unlock()

	commands, exists := types.GearCommands[gear]
	if !exists {
		return false
	}

	var selectedCommand *types.GearCommand
	for i := range commands {
		cmd := &commands[i]
		if strings.Contains(cmd.Name, level) {
			selectedCommand = cmd
			break
		}
	}

	if selectedCommand == nil {
		return false
	}

	cmdWithReportID := append([]byte{0x02}, selectedCommand.Command...)
	m.deviceOpMutex.Lock()
	_, err := dev.Write(cmdWithReportID)
	m.deviceOpMutex.Unlock()
	return err == nil
}

func (m *Manager) SetGearLight(enabled bool) bool {
	m.mutex.Lock()
	if !m.isConnected || m.device == nil {
		m.mutex.Unlock()
		return false
	}
	dev := m.device
	m.mutex.Unlock()

	var cmd []byte
	if enabled {
		cmd = []byte{0x02, 0x5A, 0xA5, 0x48, 0x03, 0x01, 0x4C}
	} else {
		cmd = []byte{0x02, 0x5A, 0xA5, 0x48, 0x03, 0x00, 0x4B}
	}
	cmd = append(cmd, make([]byte, 23-len(cmd))...)
	m.deviceOpMutex.Lock()
	_, err := dev.Write(cmd)
	m.deviceOpMutex.Unlock()
	return err == nil
}

func (m *Manager) SetPowerOnStart(enabled bool) bool {
	m.mutex.Lock()
	if !m.isConnected || m.device == nil {
		m.mutex.Unlock()
		return false
	}
	dev := m.device
	m.mutex.Unlock()

	var cmd []byte
	if enabled {
		cmd = []byte{0x02, 0x5A, 0xA5, 0x0C, 0x03, 0x02, 0x11}
	} else {
		cmd = []byte{0x02, 0x5A, 0xA5, 0x0C, 0x03, 0x01, 0x10}
	}
	cmd = append(cmd, make([]byte, 23-len(cmd))...)
	m.deviceOpMutex.Lock()
	_, err := dev.Write(cmd)
	m.deviceOpMutex.Unlock()
	return err == nil
}

func (m *Manager) SetSmartStartStop(mode string) bool {
	m.mutex.Lock()
	if !m.isConnected || m.device == nil {
		m.mutex.Unlock()
		return false
	}
	dev := m.device
	m.mutex.Unlock()

	var cmd []byte
	switch mode {
	case "off":
		cmd = []byte{0x02, 0x5A, 0xA5, 0x0D, 0x03, 0x00, 0x10}
	case "immediate":
		cmd = []byte{0x02, 0x5A, 0xA5, 0x0D, 0x03, 0x01, 0x11}
	case "delayed":
		cmd = []byte{0x02, 0x5A, 0xA5, 0x0D, 0x03, 0x02, 0x12}
	default:
		return false
	}
	cmd = append(cmd, make([]byte, 23-len(cmd))...)
	m.deviceOpMutex.Lock()
	_, err := dev.Write(cmd)
	m.deviceOpMutex.Unlock()
	return err == nil
}

func (m *Manager) SetBrightness(percentage int) bool {
	m.mutex.Lock()
	if !m.isConnected || m.device == nil {
		m.mutex.Unlock()
		return false
	}
	dev := m.device
	m.mutex.Unlock()

	var cmd []byte
	switch percentage {
	case 0:
		// 协议格式: [0x02][5A A5][cmdID=0x47][len=0x0D=13][payload(11字节)][CRC]
		// len=13 = content总长(含cmdID+len自身), payload=11字节, 有效数据只有首字节0x1C
		// CRC = sum(content) = sum(47+0D+1C+00*10) = 0x70
		cmd = []byte{0x02, 0x5A, 0xA5, 0x47, 0x0D, 0x1C, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x70}
	case 100:
		cmd = []byte{0x02, 0x5A, 0xA5, 0x43, 0x02, 0x45}
	default:
		m.logError("SetBrightness: 不支持的亮度值 %d，仅支持0或100", percentage)
		return false
	}
	cmd = append(cmd, make([]byte, 23-len(cmd))...)
	m.deviceOpMutex.Lock()
	_, err := dev.Write(cmd)
	m.deviceOpMutex.Unlock()
	return err == nil
}

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

func (m *Manager) logWarn(format string, v ...any) {
	if m.logger != nil {
		m.logger.Warn(format, v...)
	}
}
