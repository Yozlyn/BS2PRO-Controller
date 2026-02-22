// Package device - RGB lighting control
package device

import (
	"errors"
	"time"
)

// RGBColor RGB颜色
type RGBColor struct {
	R byte
	G byte
	B byte
}

// RGB速度常量
const (
	RGBSpeedFast   byte = 0x05
	RGBSpeedMedium byte = 0x0A
	RGBSpeedSlow   byte = 0x0F
)

// rgbChecksum 计算校验和（跳过前两字节 5A A5）
func rgbChecksum(payload []byte) byte {
	var sum uint16
	for _, b := range payload[2:] {
		sum += uint16(b)
	}
	return byte(sum & 0xFF)
}

// rgbSendCmd 发送RGB指令
func (m *Manager) rgbSendCmd(fields ...byte) error {
	if m.device == nil {
		return errors.New("设备未连接")
	}
	cmd := append([]byte{0x5A, 0xA5}, fields...)
	cmd = append(cmd, rgbChecksum(cmd))
	buf := make([]byte, 65)
	buf[0] = 0x02
	copy(buf[1:], cmd)
	_, err := m.device.Write(buf)
	return err
}

// rgbMakeF0 构造配置帧
func rgbMakeF0(mode, spd, bri byte, baseColor RGBColor) [10]byte {
	return [10]byte{0x00, 0x02, 0x00, mode, spd, bri, baseColor.R, baseColor.G, baseColor.B, 0x00}
}

// rgbApplyFrames 发送30帧画面
func (m *Manager) rgbApplyFrames(f0 [10]byte, frames [30][10]byte) error {
	m.rgbSendCmd(0x46, 0x03, 0x00)
	time.Sleep(100 * time.Millisecond)

	handshakes := [][]byte{
		{0x46, 0x03, 0x01}, {0x46, 0x03, 0x01}, {0x45, 0x02},
		{0x45, 0x03, 0x01}, {0x41, 0x02}, {0x41, 0x03, 0x01},
	}
	for _, cmd := range handshakes {
		m.rgbSendCmd(cmd...)
		time.Sleep(5 * time.Millisecond)
	}

	m.rgbSendCmd(append([]byte{0x47, 0x0D, 0x00}, f0[:]...)...)

	for i := 0; i < 30; i++ {
		m.rgbSendCmd(append([]byte{0x47, 0x0D, byte(i + 1)}, frames[i][:]...)...)
		time.Sleep(1 * time.Millisecond)
	}

	return m.rgbSendCmd(0x43, 0x03, 0x01)
}

// SetRGBSmartTemp 智能温控模式
func (m *Manager) SetRGBSmartTemp() bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if !m.isConnected || m.device == nil {
		return false
	}
	handshakes := [][]byte{
		{0x46, 0x03, 0x01}, {0x46, 0x03, 0x01}, {0x45, 0x02}, {0x45, 0x03, 0x01},
	}
	for _, cmd := range handshakes {
		m.rgbSendCmd(cmd...)
		time.Sleep(5 * time.Millisecond)
	}
	m.rgbSendCmd(0x44, 0x03, 0x01)
	m.rgbSendCmd(0x44, 0x03, 0x01)
	return true
}

// SetRGBOff 关闭RGB灯光
func (m *Manager) SetRGBOff() bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if !m.isConnected || m.device == nil {
		return false
	}
	err := m.rgbSendCmd(0x46, 0x03, 0x00)
	return err == nil
}

// SetRGBStaticSingle 单色常亮
func (m *Manager) SetRGBStaticSingle(color RGBColor, brightness byte) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if !m.isConnected || m.device == nil {
		return false
	}
	f0 := rgbMakeF0(0x00, RGBSpeedMedium, brightness, color)
	var frames [30][10]byte
	factor := float64(brightness) / 100.0
	r := byte(float64(color.R) * factor)
	g := byte(float64(color.G) * factor)
	b := byte(float64(color.B) * factor)
	targetIndices := []int{2, 5, 8, 11, 14}
	for _, idx := range targetIndices {
		frames[idx][6], frames[idx][7], frames[idx][8] = r, g, b
	}
	err := m.rgbApplyFrames(f0, frames)
	return err == nil
}

// SetRGBStaticMulti 多色常亮（3色）
func (m *Manager) SetRGBStaticMulti(colors [3]RGBColor, brightness byte) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if !m.isConnected || m.device == nil {
		return false
	}
	f0 := rgbMakeF0(0x00, RGBSpeedMedium, brightness, colors[0])
	var frames [30][10]byte
	factor := float64(brightness) / 100.0
	targetIndices := []int{2, 5, 8, 11, 14}
	for z, idx := range targetIndices {
		col := colors[(z+1)%3]
		frames[idx][6] = byte(float64(col.R) * factor)
		frames[idx][7] = byte(float64(col.G) * factor)
		frames[idx][8] = byte(float64(col.B) * factor)
	}
	err := m.rgbApplyFrames(f0, frames)
	return err == nil
}

// SetRGBRotation 旋转模式
func (m *Manager) SetRGBRotation(colors []RGBColor, speed, brightness byte) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if !m.isConnected || m.device == nil {
		return false
	}
	if len(colors) < 1 {
		return false
	}
	if len(colors) > 6 {
		colors = colors[:6]
	}
	f0 := rgbMakeF0(0x05, speed, brightness, RGBColor{R: 0, G: 0, B: 0})
	var frames [30][10]byte
	stream := make([]byte, 304)
	numColors := len(colors)
	factor := float64(brightness) / 100.0
	for chunkIdx := 0; chunkIdx < 6; chunkIdx++ {
		chunkStart := chunkIdx * 30
		for p := 0; p < 10; p++ {
			var r, g, bb byte
			if p < 6 {
				colorIdx := (p + chunkIdx) % 6
				if colorIdx < numColors {
					target := colors[colorIdx]
					r = byte(float64(target.R) * factor)
					g = byte(float64(target.G) * factor)
					bb = byte(float64(target.B) * factor)
				}
			}
			stream[chunkStart+p*3] = r
			stream[chunkStart+p*3+1] = g
			stream[chunkStart+p*3+2] = bb
		}
	}
	for k := 0; k < 304; k++ {
		if k < 4 {
			f0[6+k] = stream[k]
		} else {
			idx := k - 4
			frames[idx/10][idx%10] = stream[k]
		}
	}
	err := m.rgbApplyFrames(f0, frames)
	return err == nil
}

// SetRGBFlowing 流光模式
func (m *Manager) SetRGBFlowing(speed, brightness byte) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if !m.isConnected || m.device == nil {
		return false
	}
	flowingBase := [9][10]byte{
		{0x7f, 0x7f, 0x00, 0xff, 0x00, 0x7f, 0x7f, 0x00, 0xff, 0x00},
		{0x00, 0x7f, 0x00, 0x7f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x7f, 0x7f, 0x00},
		{0xff, 0x00, 0x7f, 0x7f, 0x00, 0xff, 0x00, 0x00, 0x7f, 0x00},
		{0x7f, 0x00, 0x00, 0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0x00, 0x7f},
		{0x7f, 0x00, 0xff, 0x00, 0x00, 0x7f, 0x00, 0x7f, 0x00, 0x00},
		{0xff, 0x00, 0x7f, 0x7f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0x00},
	}
	f0 := rgbMakeF0(0x05, speed, brightness, RGBColor{R: 0, G: 255, B: 0})
	factor := float64(brightness) / 100.0
	var frames [30][10]byte
	for i := 0; i < 30; i++ {
		src := flowingBase[i%9]
		for j := 0; j < 9; j++ {
			frames[i][j] = byte(float64(src[j]) * factor)
		}
		frames[i][9] = src[9]
	}
	err := m.rgbApplyFrames(f0, frames)
	return err == nil
}

// SetRGBBreathing 呼吸模式
func (m *Manager) SetRGBBreathing(colors []RGBColor, speed, brightness byte) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if !m.isConnected || m.device == nil {
		return false
	}
	if len(colors) == 0 {
		return false
	}
	if len(colors) > 5 {
		colors = colors[:5]
	}
	mode := byte(len(colors)*2 - 1)
	f0 := rgbMakeF0(mode, speed, brightness, RGBColor{R: 0, G: 0, B: 0})
	var frames [30][10]byte
	factor := float64(brightness) / 100.0
	var pattern [30]byte
	for i, col := range colors {
		offset := i * 6
		pattern[offset] = byte(float64(col.R) * factor)
		pattern[offset+1] = byte(float64(col.G) * factor)
		pattern[offset+2] = byte(float64(col.B) * factor)
	}
	for k := 0; k < 304; k++ {
		val := pattern[k%30]
		if k < 4 {
			f0[6+k] = val
		} else {
			idx := k - 4
			frames[idx/10][idx%10] = val
		}
	}
	err := m.rgbApplyFrames(f0, frames)
	return err == nil
}
