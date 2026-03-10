// Package fanoffset 提供自动风扇曲线偏移控制器
package fanoffset

import (
	"sync"
	"time"

	"github.com/TIANLI0/BS2PRO-Controller/internal/types"
)

// Controller 自动偏移控制器
// 每个温度控制点独立维护偏移状态和收敛判定
type Controller struct {
	mu sync.Mutex

	// 全局温度历史
	tempHistory []tempSample
	windowSize  int

	// 每个温度区间的运行状态
	zones []zoneState

	// 上一次温度快照，用于检测突变
	lastTemp int

	config Config
	logger types.Logger
}

type tempSample struct {
	temp      int
	timestamp time.Time
}

// zoneState 每个温度控制点的独立状态
type zoneState struct {
	stableCount  int       // 连续无偏移变化的轮次
	converged    bool      // 是否已收敛（找到最优偏移）
	convergedAt  time.Time // 收敛时刻（用于过期检测）
	lastAdjustAt time.Time // 上一次调整偏移的时刻（用于冷却期）
}

// Config 自动偏移控制器配置
type Config struct {
	// StableDelta: 温度变化在此范围内视为稳定 (默认 2°C)
	StableDelta int
	// RiseFastDelta: 温度快速上升阈值 (默认 5°C)
	RiseFastDelta int
	// SpikeThreshold: 温度突变阈值，单次采样跳跃超过此值触发紧急处理 (默认 8°C)
	SpikeThreshold int
	// MaxOffset: 每个控制点最大正偏移 (默认 500 RPM)
	MaxOffset int
	// MinOffset: 每个控制点最大负偏移 (默认 -300 RPM)，注意是负值
	MinOffset int
	// Step: 每次调整步长 (默认 100 RPM)
	Step int
	// WindowSize: 温度历史窗口大小 (默认 5 个采样)
	WindowSize int
	// HighTempThreshold: 高温阈值，超过此温度优先加强散热 (默认 80°C)
	HighTempThreshold int
	// LowTempThreshold: 低温阈值，低于此温度可降速节能 (默认 45°C)
	LowTempThreshold int
	// RPMHighRatio: 有效RPM超过最大RPM的此比例视为"高RPM" (默认 0.85)
	RPMHighRatio float64
	// ConvergeCount: 连续多少次无变化视为收敛 (默认 3)
	ConvergeCount int
	// ConvergeExpiry: 收敛状态最长持续时间，防止环境缓变时永久锁死 (默认 5分钟)
	ConvergeExpiry time.Duration
	// AdjustCooldown: 同一区间两次偏移调整的最小间隔，给物理散热留反应时间 (默认 3秒)
	AdjustCooldown time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		StableDelta:       2,
		RiseFastDelta:     5,
		SpikeThreshold:    8,
		MaxOffset:         500,
		MinOffset:         -300,
		Step:              100,
		WindowSize:        5,
		HighTempThreshold: 80,
		LowTempThreshold:  45,
		RPMHighRatio:      0.85,
		ConvergeCount:     3,
		ConvergeExpiry:    5 * time.Minute,
		AdjustCooldown:    3 * time.Second,
	}
}

// New 创建自动偏移控制器
func New(cfg Config, logger types.Logger) *Controller {
	if cfg.WindowSize < 2 {
		cfg.WindowSize = 5
	}
	if cfg.Step <= 0 {
		cfg.Step = 100
	}
	if cfg.MaxOffset <= 0 {
		cfg.MaxOffset = 500
	}
	if cfg.MinOffset > 0 {
		cfg.MinOffset = -300
	}
	if cfg.RPMHighRatio <= 0 || cfg.RPMHighRatio > 1 {
		cfg.RPMHighRatio = 0.85
	}
	if cfg.ConvergeCount <= 0 {
		cfg.ConvergeCount = 3
	}
	if cfg.SpikeThreshold <= 0 {
		cfg.SpikeThreshold = 8
	}
	if cfg.ConvergeExpiry <= 0 {
		cfg.ConvergeExpiry = 5 * time.Minute
	}
	if cfg.AdjustCooldown <= 0 {
		cfg.AdjustCooldown = 3 * time.Second
	}
	return &Controller{
		config:      cfg,
		windowSize:  cfg.WindowSize,
		tempHistory: make([]tempSample, 0, cfg.WindowSize),
		lastTemp:    -1,
		logger:      logger,
	}
}

// Update 根据当前温度和转速状态，调整对应温度区间的偏移量
// 直接修改 fanCurve[i].Offset（持久化值）
//
// 参数:
// - currentTemp: 当前温度 (°C)
// - fanCurve: 风扇曲线（会被直接修改 Offset 字段）
// - minRPM: 设备最低RPM (通常 1000)
// - maxRPM: 设备最高RPM (受挡位限制)
//
// 返回: true 表示有偏移值变动，调用方应持久化配置
func (c *Controller) Update(currentTemp int, fanCurve []types.FanCurvePoint, minRPM, maxRPM int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(fanCurve) < 2 {
		return false
	}

	// 确保 zones 数组与曲线点数匹配
	c.ensureZones(len(fanCurve))

	now := time.Now()

	// 突变检测
	// 单次采样温度跳跃过大时，唤醒相关区间的收敛状态
	if c.lastTemp >= 0 {
		tempDelta := currentTemp - c.lastTemp
		if abs(tempDelta) >= c.config.SpikeThreshold {
			c.handleSpike(currentTemp, tempDelta, fanCurve)
		}
	}
	c.lastTemp = currentTemp

	// 记录温度历史
	c.tempHistory = append(c.tempHistory, tempSample{
		temp:      currentTemp,
		timestamp: now,
	})
	if len(c.tempHistory) > c.windowSize {
		c.tempHistory = c.tempHistory[len(c.tempHistory)-c.windowSize:]
	}

	// 需要至少2个采样才能判断趋势
	if len(c.tempHistory) < 2 {
		return false
	}

	// 找到当前温度所在的区间
	zoneIdx := c.findZoneIndex(currentTemp, fanCurve)
	zone := &c.zones[zoneIdx]
	point := &fanCurve[zoneIdx]

	// 收敛过期检测
	// 防止环境缓变，时区间永久锁死
	if zone.converged {
		if now.Sub(zone.convergedAt) >= c.config.ConvergeExpiry {
			zone.converged = false
			zone.stableCount = 0
			if c.logger != nil {
				c.logger.Debug("偏移区间 %d°C 收敛过期(%v)，重新评估", point.Temperature, c.config.ConvergeExpiry)
			}
		} else {
			return false
		}
	}

	// 调整冷却期
	// 风扇提速/降速后需等待物理散热响应，避免因热惯性导致偏移过冲
	if !zone.lastAdjustAt.IsZero() && now.Sub(zone.lastAdjustAt) < c.config.AdjustCooldown {
		return false
	}

	trend := c.calculateTrend()
	effectiveRPM := point.RPM + point.Offset
	highRPMLine := int(float64(maxRPM) * c.config.RPMHighRatio)
	oldOffset := point.Offset

	// 决策矩阵
	switch {

	// 温度快速上升 + RPM有空间 → 加速散热
	case trend >= c.config.RiseFastDelta && effectiveRPM < highRPMLine:
		c.adjustZoneOffset(point, c.config.Step, minRPM, maxRPM)

	// 温度快速上升 + RPM已满 → 保持
	case trend >= c.config.RiseFastDelta && effectiveRPM >= highRPMLine:
		// RPM已接近上限，加偏移无意义

	// 温度缓慢上升 + RPM有空间 → 预加偏移
	case trend > c.config.StableDelta && effectiveRPM < highRPMLine:
		c.adjustZoneOffset(point, c.config.Step, minRPM, maxRPM)

	// 温度缓慢上升 + RPM已满 → 保持
	case trend > c.config.StableDelta && effectiveRPM >= highRPMLine:
		// 不调整

	// 温度稳定 + 高温(≥阈值) + RPM有空间 → 安全裕度不够，继续加
	case abs(trend) <= c.config.StableDelta && currentTemp >= c.config.HighTempThreshold && effectiveRPM < highRPMLine:
		c.adjustZoneOffset(point, c.config.Step, minRPM, maxRPM)

	// 温度稳定 + 高温 + RPM已满 → 保持
	case abs(trend) <= c.config.StableDelta && currentTemp >= c.config.HighTempThreshold:
		// 不调整

	// 温度稳定 + 非高温 → 保持当前偏移，让收敛计数增长
	case abs(trend) <= c.config.StableDelta:
		// 不调整 — 稳态即最优，等待收敛锁定

	// 温度明确下降 → 当前转速过剩，减偏移（直到 MinOffset）
	case trend < -c.config.StableDelta:
		c.adjustZoneOffset(point, -c.config.Step, minRPM, maxRPM)

	default:
		// 其他情况不调整
	}

	// 收敛判定（动态轮次：偏移越大，需要越多轮稳定确认）
	if point.Offset == oldOffset {
		zone.stableCount++
		requiredCount := abs(point.Offset) / c.config.Step
		if requiredCount < c.config.ConvergeCount {
			requiredCount = c.config.ConvergeCount
		}
		if zone.stableCount >= requiredCount {
			zone.converged = true
			zone.convergedAt = now
			if c.logger != nil {
				c.logger.Debug("风扇偏移区间 %d°C 已收敛，偏移=%d RPM", point.Temperature, point.Offset)
			}
		}
		return false
	}

	// 偏移有变化，重置收敛计数，记录调整时间
	zone.stableCount = 0
	zone.lastAdjustAt = now
	return true
}

// handleSpike 处理温度突变
// 唤醒起点到终点之间经过的所有区间，避免跨区遗漏
func (c *Controller) handleSpike(currentTemp, tempDelta int, fanCurve []types.FanCurvePoint) {
	prevTemp := currentTemp - tempDelta
	startIdx := c.findZoneIndex(prevTemp, fanCurve)
	endIdx := c.findZoneIndex(currentTemp, fanCurve)

	// 确保 lo <= hi
	lo, hi := startIdx, endIdx
	if lo > hi {
		lo, hi = hi, lo
	}

	// 额外向外扩展1个区间，覆盖边界效应
	if lo > 0 {
		lo--
	}
	if hi < len(c.zones)-1 {
		hi++
	}

	for i := lo; i <= hi; i++ {
		c.zones[i].converged = false
		c.zones[i].stableCount = 0
	}

	if c.logger != nil {
		c.logger.Debug("温度突变 %d°C→%d°C (%+d)，唤醒区间 %d°C-%d°C",
			prevTemp, currentTemp, tempDelta,
			fanCurve[lo].Temperature, fanCurve[hi].Temperature)
	}

	// 突变后清空温度历史，避免旧数据干扰趋势判断
	c.tempHistory = c.tempHistory[:0]
}

// Reset 重置运行状态（温度历史、收敛标记），不清除偏移值
// 偏移值存在 FanCurvePoint.Offset 里，由配置持久化
func (c *Controller) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tempHistory = c.tempHistory[:0]
	c.lastTemp = -1
	for i := range c.zones {
		c.zones[i] = zoneState{}
	}
}

// ResetOffsets 彻底重置：清除所有偏移值并重置运行状态
// 仅在用户主动关闭偏移功能时调用
func (c *Controller) ResetOffsets(fanCurve []types.FanCurvePoint) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tempHistory = c.tempHistory[:0]
	c.lastTemp = -1
	for i := range c.zones {
		c.zones[i] = zoneState{}
	}
	for i := range fanCurve {
		fanCurve[i].Offset = 0
	}
}

// GetZoneStatus 获取所有区间的收敛状态
func (c *Controller) GetZoneStatus() []bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]bool, len(c.zones))
	for i, z := range c.zones {
		result[i] = z.converged
	}
	return result
}

// ensureZones 确保 zones 数组与曲线点数一致
func (c *Controller) ensureZones(n int) {
	if len(c.zones) == n {
		return
	}
	c.zones = make([]zoneState, n)
}

// findZoneIndex 找到当前温度所在的区间索引
func (c *Controller) findZoneIndex(temp int, fanCurve []types.FanCurvePoint) int {
	if temp <= fanCurve[0].Temperature {
		return 0
	}
	for i := len(fanCurve) - 1; i >= 0; i-- {
		if fanCurve[i].Temperature <= temp {
			return i
		}
	}
	return 0
}

// adjustZoneOffset 调整单个控制点的偏移量并钳位
func (c *Controller) adjustZoneOffset(point *types.FanCurvePoint, delta, minRPM, maxRPM int) {
	point.Offset += delta

	// 钳位到配置范围
	if point.Offset > c.config.MaxOffset {
		point.Offset = c.config.MaxOffset
	}
	if point.Offset < c.config.MinOffset {
		point.Offset = c.config.MinOffset
	}

	// 确保为Step的整数倍
	if c.config.Step > 0 {
		point.Offset = (point.Offset / c.config.Step) * c.config.Step
	}

	// 安全边界：确保最终RPM在合法范围内
	finalRPM := point.RPM + point.Offset
	if finalRPM < minRPM {
		point.Offset = minRPM - point.RPM
		point.Offset = (point.Offset / c.config.Step) * c.config.Step
	}
	if finalRPM > maxRPM {
		point.Offset = maxRPM - point.RPM
		point.Offset = (point.Offset / c.config.Step) * c.config.Step
	}
}

// calculateTrend 计算温度趋势
func (c *Controller) calculateTrend() int {
	if len(c.tempHistory) < 2 {
		return 0
	}
	return c.tempHistory[len(c.tempHistory)-1].temp - c.tempHistory[0].temp
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
