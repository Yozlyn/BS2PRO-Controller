// Package rgb 提供独立彻底解耦的 RGB 灯光控制及协议传输
package rgb

import (
	"time"
)

// RGB 速度常量
const (
	SpeedFast   = 5
	SpeedMedium = 15
	SpeedSlow   = 30
)

// 协议指令常量
const (
	CmdPrepare     = 0x41 // 传输准备指令
	CmdTransport   = 0x47 // 数据分包传输
	CmdFinish      = 0x43 // 传输完成标志
	CmdSetState    = 0x46 // 开关状态
	CmdIntelligent = 0x44 // 智能模式
)

// 数据包相关设置
const (
	chunkSize     = 10  // 单个分包有效载荷大小
	configLen     = 306 // 矩阵配置总长度
	colorGroupLen = 30  // 单个颜色组长度
)

// ACK 超时设置
const (
	// sendConfig 约需 31包 × 3ms = 93ms，再加 prepare/finish 各一次等待，
	// 留足余量避免智能变频并发时 ACK 被抢占。
	ackTimeoutShort = 300 * time.Millisecond // 单指令 ACK 超时（原150ms→300ms）
	ackTimeoutLong  = 600 * time.Millisecond // finish 指令 ACK 超时（数据量大，硬件处理更久）
)

// Color 表示单个RGB颜色
type Color struct {
	R, G, B byte
}

// Transport 定义了控制器如何与下层硬件通讯的接口
type Transport interface {
	// WritePacket 仅发送数据，不等待ACK (用于解决批量分包导致的6秒卡顿)
	WritePacket(packet []byte) error
	// WritePacketAndWaitACK 发送数据并等待确认 (用于关键控制指令)
	WritePacketAndWaitACK(cmdID byte, packet []byte, timeout time.Duration) bool
}

// Controller 控制高级别的 RGB 灯效下发
type Controller struct {
	tr Transport
	// 用 channel 实现可超时的互斥锁，容量为1代表锁未被持有。
	// 相比 sync.Mutex 优势：TryLock 和带超时的 Lock 均可原生实现。
	cmdSem chan struct{}

	// 异步智能控温使用的通道
	cmdQueue chan byte
	stopChan chan struct{}
}

// NewController 创建一个独立的 RGB 控制器
func NewController(tr Transport) *Controller {
	sem := make(chan struct{}, 1)
	sem <- struct{}{} // 初始时放入令牌，代表锁可用
	return &Controller{
		tr:       tr,
		cmdSem:   sem,
		cmdQueue: make(chan byte, 5),
	}
}

// lockWithTimeout 带超时地获取锁，适用于用户主动操作（最多等待1秒）。
// 返回 false 表示设备忙，调用方应向用户反馈失败而非无限阻塞。
func (c *Controller) lockWithTimeout() bool {
	select {
	case <-c.cmdSem:
		return true
	case <-time.After(1 * time.Second):
		return false
	}
}

// tryLock 非阻塞地尝试获取锁，失败时直接返回 false。
// 专用于后台智能温控：拿不到锁说明用户正在操作，直接跳过本次更新。
func (c *Controller) tryLock() bool {
	select {
	case <-c.cmdSem:
		return true
	default:
		return false
	}
}

// unlock 释放锁
func (c *Controller) unlock() {
	c.cmdSem <- struct{}{}
}

// Start 开启后台队列工作器 (用于平滑下发智能温控)
func (c *Controller) Start() {
	// 修复: 先Stop再重建，Stop内部已有nil保护，避免直接close导致
	// 对已关闭channel的二次close panic
	c.Stop()
	c.stopChan = make(chan struct{})

	go func() {
		var lastSend time.Time
		for {
			select {
			case <-c.stopChan:
				return
			case level := <-c.cmdQueue:
				// 防抖: 控制命令频率
				if time.Since(lastSend) > 2*time.Second {
					c.SetSmartTempLevel(level)
					lastSend = time.Now()
				}
			}
		}
	}()
}

// Stop 停止工作器
func (c *Controller) Stop() {
	if c.stopChan != nil {
		close(c.stopChan)
		c.stopChan = nil
	}
}

// buildPacket 封装 RGB 协议底层包头包尾及校验: [5A A5 cmdID len payload... crc]
func buildPacket(cmdID byte, payload []byte) []byte {
	cLen := 2
	if payload != nil {
		cLen += len(payload)
	}

	content := make([]byte, cLen)
	content[0] = cmdID
	content[1] = byte(cLen)
	if payload != nil {
		copy(content[2:], payload)
	}

	var crc byte
	for _, b := range content {
		crc += b
	}

	packet := make([]byte, 2+len(content)+1)
	packet[0] = 0x5A
	packet[1] = 0xA5
	copy(packet[2:], content)
	packet[len(packet)-1] = crc

	return packet
}

// setState 硬件灯光开关（调用方须持有 cmdSem 令牌）
func (c *Controller) setState(on bool) bool {
	payload := []byte{0x00}
	if on {
		payload[0] = 0x01
	}
	pkt := buildPacket(CmdSetState, payload)
	return c.tr.WritePacketAndWaitACK(CmdSetState, pkt, ackTimeoutShort)
}

// sendConfig 发送完整矩阵配置（解决过慢问题的核心所在，调用方须持有 cmdSem 令牌）
func (c *Controller) sendConfig(cfg *rgbConfig) bool {
	data := cfg.Bytes()

	// 1. 发送准备指令，最多重试3次（参考原始固件协议重试逻辑）
	// Prepare 失败说明硬件未就绪，继续发数据包没有意义
	preparePkt := buildPacket(CmdPrepare, nil)
	prepared := false
	for i := 0; i < 3; i++ {
		if c.tr.WritePacketAndWaitACK(CmdPrepare, preparePkt, ackTimeoutShort) {
			prepared = true
			break
		}
	}
	if !prepared {
		return false
	}

	// 2. 连续发送数据包，不再强制等待硬件确认 (Fire and forget!)
	// 这将使得传输耗时从 6秒骤减至 < 0.1秒
	totalChunks := (len(data) + chunkSize - 1) / chunkSize
	for i := 0; i < totalChunks; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(data) {
			end = len(data)
		}

		payload := make([]byte, len(data[start:end])+1)
		payload[0] = byte(i)
		copy(payload[1:], data[start:end])

		pkt := buildPacket(CmdTransport, payload)
		_ = c.tr.WritePacket(pkt)

		// 给 MCU 喘息的时间（3毫秒即可），防止底层缓冲区溢出
		time.Sleep(3 * time.Millisecond)
	}

	// 3. 发送结束指令，最多重试3次
	// Finish 的 ACK 代表硬件已完整接收并应用配置，是真正的成功标志
	finishPkt := buildPacket(CmdFinish, nil)
	for i := 0; i < 3; i++ {
		if c.tr.WritePacketAndWaitACK(CmdFinish, finishPkt, ackTimeoutLong) {
			return true
		}
	}
	return false
}

// --- 以下为对外部暴露的灯效设置方法 ---

func (c *Controller) SetFlowing(speed, brightness byte) bool {
	if !c.lockWithTimeout() {
		return false // 设备忙，用户操作无法在1秒内开始
	}
	defer c.unlock()
	cfg := newRGBConfig()
	cfg.SetStreamer()
	cfg.LoopTime = speed
	cfg.LightScale = brightness
	if !c.sendConfig(cfg) {
		return false
	}
	return c.setState(true)
}

func (c *Controller) SetRotation(colors []Color, speed, brightness byte) bool {
	if !c.lockWithTimeout() {
		return false
	}
	defer c.unlock()
	cfg := newRGBConfig()
	cfg.SetRotate(colors)
	cfg.LoopTime = speed
	cfg.LightScale = brightness
	if !c.sendConfig(cfg) {
		return false
	}
	return c.setState(true)
}

func (c *Controller) SetBreathing(colors []Color, speed, brightness byte) bool {
	if !c.lockWithTimeout() {
		return false
	}
	defer c.unlock()
	cfg := newRGBConfig()
	cfg.SetBreathe(colors)
	cfg.LoopTime = speed
	cfg.LightScale = brightness
	if !c.sendConfig(cfg) {
		return false
	}
	return c.setState(true)
}

func (c *Controller) SetStaticSingle(color Color, brightness byte) bool {
	if !c.lockWithTimeout() {
		return false
	}
	defer c.unlock()
	cfg := newRGBConfig()
	cfg.SetPure(color)
	cfg.LightScale = brightness
	if !c.sendConfig(cfg) {
		return false
	}
	return c.setState(true)
}

func (c *Controller) SetStaticMulti(colors [3]Color, brightness byte) bool {
	if !c.lockWithTimeout() {
		return false
	}
	defer c.unlock()
	cfg := newRGBConfig()
	cfg.SetMulticolor(colors[:])
	cfg.LightScale = brightness
	if !c.sendConfig(cfg) {
		return false
	}
	return c.setState(true)
}

func (c *Controller) SetSmartTempLevel(level byte) bool {
	// 后台调用：拿不到锁说明用户正在操作，直接跳过本次温控更新
	if !c.tryLock() {
		return false
	}
	defer c.unlock()
	if !c.setState(true) {
		return false
	}
	pkt := buildPacket(CmdIntelligent, []byte{level})
	return c.tr.WritePacketAndWaitACK(CmdIntelligent, pkt, ackTimeoutShort)
}

func (c *Controller) AsyncSetSmartTempLevel(level byte) {
	if level < 1 || level > 4 {
		return
	}
	select {
	case c.cmdQueue <- level:
	default:
	}
}

func (c *Controller) SetOff() bool {
	if !c.lockWithTimeout() {
		return false
	}
	defer c.unlock()
	return c.setState(false)
}

// ============================================
// 灯光矩阵模型与插值算法
// ============================================

type rgbGroup struct{ Units [10][3]byte }

func (g *rgbGroup) Set(i int, c Color) {
	if i >= 0 && i < 10 {
		g.Units[i] = [3]byte{c.R, c.G, c.B}
	}
}

func (g *rgbGroup) Bytes() []byte {
	b := make([]byte, colorGroupLen)
	for i, u := range g.Units {
		b[i*3], b[i*3+1], b[i*3+2] = u[0], u[1], u[2]
	}
	return b
}

func (g *rgbGroup) clear() {
	for i := 0; i < 10; i++ {
		g.Set(i, Color{0, 0, 0})
	}
}

type rgbConfig struct {
	Version    [2]byte
	LoopStart  byte
	LoopEnd    byte
	LoopTime   byte
	LightScale byte
	Id         [10]rgbGroup
}

func newRGBConfig() *rgbConfig {
	return &rgbConfig{
		Version:    [2]byte{0, 2},
		LoopStart:  0,
		LoopEnd:    1,
		LoopTime:   15,
		LightScale: 100,
	}
}

func (c *rgbConfig) Bytes() []byte {
	buf := make([]byte, configLen)
	buf[0], buf[1] = c.Version[0], c.Version[1]
	buf[2], buf[3], buf[4], buf[5] = c.LoopStart, c.LoopEnd, c.LoopTime, c.LightScale
	for i := 0; i < 10; i++ {
		copy(buf[6+i*colorGroupLen:], c.Id[i].Bytes())
	}
	return buf
}

func (c *rgbConfig) clear() {
	for i := range c.Id {
		c.Id[i].clear()
	}
}

func (c *rgbConfig) SetStreamer() {
	c.LoopStart, c.LoopEnd = 0, 5
	c.clear()

	blue, cyan := Color{0, 0, 255}, Color{0, 127, 127}
	green, yellow := Color{0, 255, 0}, Color{127, 127, 0}
	red, magenta := Color{255, 0, 0}, Color{127, 0, 127}

	row0 := []Color{blue, cyan, green, yellow, red, magenta}
	row1 := []Color{cyan, green, yellow, red, magenta, blue}
	row2 := []Color{green, yellow, red, magenta, blue, cyan}

	for col := 0; col < 6; col++ {
		c.Id[0].Set(col, row0[col])
		c.Id[1].Set(col, row1[col])
		c.Id[2].Set(col, row2[col])
		c.Id[3].Set(col, row0[col])
		c.Id[4].Set(col, row1[col])
		c.Id[5].Set(col, row2[col])
	}
}

func (c *rgbConfig) SetRotate(colors []Color) {
	if len(colors) == 0 {
		colors = []Color{{255, 0, 0}, {0, 255, 0}, {0, 0, 255}}
	}
	c.LoopStart, c.LoopEnd = 0, 5
	c.clear()
	for i := 0; i < 6; i++ {
		for ci, col := range colors {
			c.Id[i].Set((6+ci-i)%6, col)
		}
	}
}

func (c *rgbConfig) SetBreathe(colors []Color) {
	if len(colors) == 0 {
		colors = []Color{{0, 0, 255}}
	}
	n := byte(len(colors)*2 - 1)
	if n > 9 {
		n = 9
	}
	c.LoopEnd = n
	c.clear()

	maxUnits := len(colors) * 2
	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			if j >= maxUnits {
				c.Id[i].Set(j, Color{0, 0, 0})
			} else if j%2 == 0 {
				c.Id[i].Set(j, Color{0, 0, 0})
			} else {
				c.Id[i].Set(j, colors[(j/2)%len(colors)])
			}
		}
	}
}

func (c *rgbConfig) SetPure(color Color) {
	c.LoopEnd = 0
	c.clear()
	for i := 0; i < 10; i++ {
		c.Id[i].Set(0, color)
	}
}

func (c *rgbConfig) SetMulticolor(colors []Color) {
	if len(colors) == 0 {
		colors = []Color{{255, 0, 0}, {0, 255, 0}, {0, 0, 255}}
	}
	c.LoopStart, c.LoopEnd = 0, 0
	c.clear()
	n := len(colors)
	if n > 3 {
		n = 3
	}
	for j := 0; j < n; j++ {
		c.Id[j].Set(0, colors[j])
		c.Id[j+3].Set(0, colors[j])
	}
}
