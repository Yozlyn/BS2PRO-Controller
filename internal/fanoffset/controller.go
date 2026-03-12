// Package fanoffset 提供自动风扇曲线偏移控制器
package fanoffset

import (
	"math"
	"sync"
	"time"

	"github.com/TIANLI0/BS2PRO-Controller/internal/types"
)

// Controller 自动偏移控制器
type Controller struct {
	mu sync.Mutex

	tempHistory []tempSample
	windowSize  int
	zones       []zoneState
	lastTemp    int

	config Config
	logger types.Logger
}

type tempSample struct {
	temp int
}

// zoneState 每个温度控制点的独立运行状态
type zoneState struct {
	lastAdjustAt time.Time

	// 稳定轮次计数，达到 ConvergeCount 后触发一次重力探测
	stableCount int

	// 重力探测：探一步，下一轮观察温度是否响应
	probeActive bool
	probeOffset int  // 探测前的 offset，若温度响应则恢复此值
	probeUp     bool // true=向上探测(高温区重力), false=向下探测(低温区重力)

	// 收敛观察期：探测触底 / 恢复后，持续观察温度稳定性
	verifying      bool
	verifyStartAt  time.Time
	verifyTempHigh int
	verifyTempLow  int

	// 已永久收敛
	converged   bool
	convergeTmp int
	driftCount  int

	// 高温区连续趋势计数：防止单轮抖动触发调整
	trendUpCount   int // 连续上升趋势轮次
	trendDownCount int // 连续下降趋势轮次
}

// Config 控制器配置
type Config struct {
	// StableDelta: ±N°C 以内视为"稳定"(默认 2)
	StableDelta int
	// SpikeThreshold: 单次采样温度跳跃超过此值触发强制唤醒 (默认 15°C)
	SpikeThreshold int
	// Step: 每次偏移调整步长 RPM (默认 100)
	Step int
	// WindowSize: 温度历史窗口（个采样）(默认 5)
	WindowSize int
	// ConvergeCount: 连续稳定多少轮触发一次重力探测 (默认 3)
	ConvergeCount int
	// VerifyDuration: 收敛观察期时长 (默认 30s)
	VerifyDuration time.Duration
	// VerifyMaxDelta: 观察期内温度波动上限 (默认 3°C)
	VerifyMaxDelta int
	// UpwardDriftThreshold: 收敛后温度上升超过此值触发漂移计数 (默认 5°C)
	UpwardDriftThreshold int
	// DownwardDriftThreshold: 收敛后温度下降超过此值触发漂移计数，应为负值 (默认 -10°C)
	DownwardDriftThreshold int
	// DriftCount: 连续多少次漂移才解锁 (默认 20)
	DriftCount int
	// AdjustCooldown: 两次偏移调整之间的最小间隔 (默认 5s)
	AdjustCooldown time.Duration
	// HighTempTrendConfirm: 高温区间连续多少轮趋势一致才调整偏移，防止热振荡引起转速跳变 (默认 2)
	HighTempTrendConfirm int
	// HighTempThreshold: 高于此温度（°C）视为高温区，使用向上重力策略 (默认 60)
	HighTempThreshold int
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		StableDelta:            2,
		SpikeThreshold:         15,
		Step:                   100,
		WindowSize:             5,
		ConvergeCount:          3,
		VerifyDuration:         30 * time.Second,
		VerifyMaxDelta:         3,
		UpwardDriftThreshold:   5,
		DownwardDriftThreshold: -10,
		DriftCount:             20,
		AdjustCooldown:         5 * time.Second,
		HighTempTrendConfirm:   2,
		HighTempThreshold:      60,
	}
}

// New 创建偏移控制器
func New(cfg Config, logger types.Logger) *Controller {
	if cfg.WindowSize < 2 {
		cfg.WindowSize = 5
	}
	if cfg.Step <= 0 {
		cfg.Step = 100
	}
	if cfg.ConvergeCount <= 0 {
		cfg.ConvergeCount = 3
	}
	if cfg.SpikeThreshold <= 0 {
		cfg.SpikeThreshold = 15
	}
	if cfg.VerifyDuration <= 0 {
		cfg.VerifyDuration = 30 * time.Second
	}
	if cfg.VerifyMaxDelta <= 0 {
		cfg.VerifyMaxDelta = 3
	}
	if cfg.UpwardDriftThreshold <= 0 {
		cfg.UpwardDriftThreshold = 5
	}
	if cfg.DownwardDriftThreshold >= 0 {
		cfg.DownwardDriftThreshold = -10
	}
	if cfg.DriftCount <= 0 {
		cfg.DriftCount = 20
	}
	if cfg.AdjustCooldown <= 0 {
		cfg.AdjustCooldown = 5 * time.Second
	}
	if cfg.HighTempTrendConfirm <= 0 {
		cfg.HighTempTrendConfirm = 2
	}
	if cfg.HighTempThreshold <= 0 {
		cfg.HighTempThreshold = 60
	}
	return &Controller{
		config:      cfg,
		windowSize:  cfg.WindowSize,
		tempHistory: make([]tempSample, 0, cfg.WindowSize),
		lastTemp:    -1,
		logger:      logger,
	}
}

// Update 根据当前温度调整对应区间的偏移量
func (c *Controller) Update(currentTemp int, fanCurve []types.FanCurvePoint, minRPM, maxRPM int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(fanCurve) < 2 {
		return false
	}

	c.ensureZones(fanCurve)
	now := time.Now()

	// 突变检测：单次跳跃过大，唤醒相关区间
	if c.lastTemp >= 0 {
		if d := currentTemp - c.lastTemp; iabs(d) >= c.config.SpikeThreshold {
			c.handleSpike(currentTemp, d, fanCurve)
		}
	}
	c.lastTemp = currentTemp

	// 追加温度历史
	c.tempHistory = append(c.tempHistory, tempSample{temp: currentTemp})
	if len(c.tempHistory) > c.windowSize {
		c.tempHistory = c.tempHistory[len(c.tempHistory)-c.windowSize:]
	}
	if len(c.tempHistory) < 2 {
		return false
	}

	zoneIdx := c.findZoneIndex(currentTemp, fanCurve)
	zone := &c.zones[zoneIdx]
	point := &fanCurve[zoneIdx]

	// 已收敛：仅做漂移检测
	if zone.converged {
		c.checkDrift(zone, currentTemp, point)
		return false
	}

	// 提前计算趋势，后续所有分支复用
	trend := c.calculateTrend()

	// 收敛观察期
	if zone.verifying {
		return c.handleVerifying(zone, point, currentTemp, trend, now, minRPM, maxRPM)
	}

	// 重力探测响应期
	// 上一轮对这个区间发出了探测，本轮判断温度是否有反应
	if zone.probeActive {
		zone.probeActive = false

		if zone.probeUp {
			// 高温区向上探测
			// 加了转速后温度是否下降？是 → 有效，保留并继续；否 → 无益，恢复
			responseThreshold := -c.config.StableDelta
			if zoneIdx == len(fanCurve)-1 {
				responseThreshold = -c.config.StableDelta * 2
			}
			if trend <= responseThreshold {
				// 温度下降 → 加速有效，保留新偏移，继续向上探
				zone.stableCount = 0
				if c.logger != nil {
					c.logger.Debug("区间 %d°C 向上探测有效: offset=%d 温度下降，继续探测",
						point.Temperature, point.Offset)
				}
			} else {
				// 温度未下降 → 额外转速无益，恢复偏移，进入观察期
				oldOffset := point.Offset
				point.Offset = zone.probeOffset
				zone.lastAdjustAt = now
				c.startVerifying(zone, currentTemp, now)
				if c.logger != nil {
					c.logger.Debug("区间 %d°C 向上探测无效: offset %d→%d, 进入观察期",
						point.Temperature, oldOffset, point.Offset)
				}
				return point.Offset != oldOffset
			}
		} else {
			// 低温区向下探测
			// 降了转速后温度是否上升？是 → 触底，恢复；否 → 安全，继续向下探
			probeResponseThreshold := c.config.StableDelta
			if zoneIdx == len(fanCurve)-1 {
				probeResponseThreshold = c.config.StableDelta * 2
			}
			if trend >= probeResponseThreshold {
				// 温度对探测产生了响应 → 恢复到探测前的偏移，进入收敛观察期
				oldOffset := point.Offset
				point.Offset = zone.probeOffset
				zone.lastAdjustAt = now
				c.startVerifying(zone, currentTemp, now)
				if c.logger != nil {
					c.logger.Debug("区间 %d°C 探测触底回弹: offset %d→%d, 进入观察期",
						point.Temperature, oldOffset, point.Offset)
				}
				return point.Offset != oldOffset
			}
			// 温度未响应 → 此探测层安全，归零稳定计数，继续向下探
			zone.stableCount = 0
			if c.logger != nil {
				c.logger.Debug("区间 %d°C 探测安全: offset=%d 无响应，继续探索",
					point.Temperature, point.Offset)
			}
		}
		// fall-through 到冷却期检查
	}

	// 冷却期
	if !zone.lastAdjustAt.IsZero() && now.Sub(zone.lastAdjustAt) < c.config.AdjustCooldown {
		return false
	}

	oldOffset := point.Offset
	spreadChanged := false
	smoothChanged := false

	// 以物理温度阈值判断高温区，兼容不同曲线节点数量
	isHighTemp := currentTemp >= c.config.HighTempThreshold

	switch {
	case trend >= c.config.StableDelta:
		// 温度上升
		zone.trendDownCount = 0
		if !isHighTemp {
			// 低温区：缓慢正偏移 —— 需连续确认上升趋势，避免短暂睿频毛刺
			zone.trendUpCount++
			if zone.trendUpCount < c.config.HighTempTrendConfirm {
				break
			}
		}
		zone.trendUpCount = 0
		// 高温区：快速正偏移 —— 立即响应，散热安全优先
		upStep := c.config.Step
		if zoneIdx == len(fanCurve)-1 {
			upStep = c.config.Step * 3
		} else if isHighTemp {
			upStep = c.config.Step * 2
		}
		c.adjustZoneOffset(point, upStep, minRPM, maxRPM)
		spreadChanged = c.spreadToLowerNeighbor(zoneIdx, fanCurve, minRPM, maxRPM, now)
		zone.stableCount = 0
		if c.logger != nil {
			c.logger.Debug("区间 %d°C 上升(trend=%d): offset %d→%d",
				point.Temperature, trend, oldOffset, point.Offset)
		}

	case trend <= -c.config.StableDelta:
		// 温度下降 → 大幅减偏移，并传播平滑。快速摸底以应对频繁跳动
		if isHighTemp {
			zone.trendDownCount++
			zone.trendUpCount = 0
			if zone.trendDownCount < c.config.HighTempTrendConfirm {
				break // 确认轮次不足，等待下一轮
			}
			zone.trendDownCount = 0
		}
		dropStep := c.config.Step
		if point.Offset > c.config.Step*5 {
			dropStep = point.Offset / 2 // 高偏离时直接砍半，快速摸底
		} else if point.Offset > c.config.Step {
			dropStep = c.config.Step * 3
		}
		c.adjustZoneOffset(point, -dropStep, minRPM, maxRPM)
		zone.stableCount = 0
		if point.Offset != oldOffset {
			// 温度真正下降时做冷端传播，防止低温区保留过高的历史偏移
			smoothChanged = c.applyCurveSmoothingAfterDescend(zoneIdx, fanCurve, minRPM, maxRPM, now)
		}
		if c.logger != nil {
			c.logger.Debug("区间 %d°C 下降(trend=%d): offset %d→%d",
				point.Temperature, trend, oldOffset, point.Offset)
		}

	default:
		// 温度稳定：重置趋势计数
		zone.trendUpCount = 0
		zone.trendDownCount = 0

		// 双向重力探测：
		//   低温区 → 向下探测（寻找最低可接受转速，快速负偏移）
		//   高温区 → 向上探测（寻找更安全的转速，快速正偏移）
		zone.stableCount++
		if zone.stableCount >= c.config.ConvergeCount {
			zone.stableCount = 0
			zone.probeOffset = point.Offset

			if isHighTemp {
				// ── 高温区：向上重力探测 ──
				zone.probeUp = true
				probeStep := c.config.Step
				c.adjustZoneOffset(point, probeStep, minRPM, maxRPM)

				if point.Offset == zone.probeOffset {
					// 已到 RPM 物理顶部 → 进入收敛观察期
					c.startVerifying(zone, currentTemp, now)
					if c.logger != nil {
						c.logger.Debug("区间 %d°C 触顶(offset=%d): 进入观察期", point.Temperature, point.Offset)
					}
				} else {
					zone.probeActive = true
					if c.logger != nil {
						c.logger.Debug("区间 %d°C 向上重力探测: offset %d→%d",
							point.Temperature, zone.probeOffset, point.Offset)
					}
				}
			} else {
				// 低温区：向下重力探测
				zone.probeUp = false
				// 快速向下探底
				probeDrop := c.config.Step
				if point.Offset > c.config.Step*4 {
					probeDrop = point.Offset / 2
				} else if point.Offset > c.config.Step {
					probeDrop = c.config.Step * 2
				}
				c.adjustZoneOffset(point, -probeDrop, minRPM, maxRPM)

				if point.Offset == zone.probeOffset {
					// 已到 RPM 物理底部 → 进入收敛观察期
					c.startVerifying(zone, currentTemp, now)
					if c.logger != nil {
						c.logger.Debug("区间 %d°C 触底(offset=%d): 进入观察期", point.Temperature, point.Offset)
					}
				} else {
					zone.probeActive = true
					if c.logger != nil {
						c.logger.Debug("区间 %d°C 重力探测: offset %d→%d",
							point.Temperature, zone.probeOffset, point.Offset)
					}
				}
			}
		}
	}

	if point.Offset != oldOffset || spreadChanged || smoothChanged {
		zone.lastAdjustAt = now
		return true
	}
	return false
}

// handleVerifying 处理收敛观察期
func (c *Controller) handleVerifying(zone *zoneState, point *types.FanCurvePoint,
	currentTemp, trend int, now time.Time, minRPM, maxRPM int) bool {

	if currentTemp > zone.verifyTempHigh {
		zone.verifyTempHigh = currentTemp
	}
	if currentTemp < zone.verifyTempLow {
		zone.verifyTempLow = currentTemp
	}

	// 观察期内温度明显上升 → 需要加偏移，退出观察
	if trend >= c.config.StableDelta {
		zone.verifying = false
		zone.stableCount = 0
		oldOffset := point.Offset
		c.adjustZoneOffset(point, c.config.Step, minRPM, maxRPM)
		zone.lastAdjustAt = now
		if c.logger != nil {
			c.logger.Debug("区间 %d°C 观察期温度上升(trend=%d): offset %d→%d, 退出观察",
				point.Temperature, trend, oldOffset, point.Offset)
		}
		return point.Offset != oldOffset
	}

	// 观察期内温度波动幅度过大 → 退出
	if zone.verifyTempHigh-zone.verifyTempLow > c.config.VerifyMaxDelta {
		zone.verifying = false
		zone.stableCount = 0
		if c.logger != nil {
			c.logger.Debug("区间 %d°C 观察期波动超限(%d°C), 退出",
				point.Temperature, zone.verifyTempHigh-zone.verifyTempLow)
		}
		return false
	}

	// 观察期未满
	if now.Sub(zone.verifyStartAt) < c.config.VerifyDuration {
		return false
	}

	// 观察期通过 → 确认收敛
	zone.verifying = false
	zone.converged = true
	zone.convergeTmp = currentTemp
	zone.stableCount = 0
	zone.driftCount = 0
	if c.logger != nil {
		c.logger.Debug("区间 %d°C 确认收敛! offset=%d RPM, 基准=%d°C",
			point.Temperature, point.Offset, currentTemp)
	}
	return false
}

// startVerifying 进入收敛观察期
func (c *Controller) startVerifying(zone *zoneState, currentTemp int, now time.Time) {
	zone.verifying = true
	zone.verifyStartAt = now
	zone.verifyTempHigh = currentTemp
	zone.verifyTempLow = currentTemp
}

// checkDrift 收敛后漂移检测
func (c *Controller) checkDrift(zone *zoneState, currentTemp int, point *types.FanCurvePoint) {
	delta := currentTemp - zone.convergeTmp
	if delta >= c.config.UpwardDriftThreshold || delta <= c.config.DownwardDriftThreshold {
		zone.driftCount++
		if zone.driftCount >= c.config.DriftCount {
			zone.converged = false
			zone.stableCount = 0
			zone.driftCount = 0
			zone.verifying = false
			if c.logger != nil {
				c.logger.Debug("区间 %d°C 漂移解锁: 当前%d°C 基准%d°C (delta: %d) 连续%d次",
					point.Temperature, currentTemp, zone.convergeTmp, delta, c.config.DriftCount)
			}
		}
	} else {
		zone.driftCount = 0
	}
}

// applyCurveSmoothingAfterDescend 某区间 RPM 确认下降后调用，执行冷端传播
func (c *Controller) applyCurveSmoothingAfterDescend(changedIdx int, fanCurve []types.FanCurvePoint, minRPM, maxRPM int, now time.Time) bool {
	effectAtChanged := fanCurve[changedIdx].RPM + fanCurve[changedIdx].Offset
	changed := false

	for j := changedIdx - 1; j >= 0; j-- {
		p := &fanCurve[j]
		eff := p.RPM + p.Offset
		if eff <= effectAtChanged {
			continue
		}
		targetOffset := effectAtChanged - p.RPM
		if c.config.Step > 0 {
			targetOffset = int(math.Floor(float64(targetOffset)/float64(c.config.Step))) * c.config.Step
		}
		// 物理边界保护
		if p.RPM+targetOffset < minRPM {
			targetOffset = int(math.Ceil(float64(minRPM-p.RPM)/float64(c.config.Step))) * c.config.Step
		} else if p.RPM+targetOffset > maxRPM {
			targetOffset = int(math.Floor(float64(maxRPM-p.RPM)/float64(c.config.Step))) * c.config.Step
		}
		if p.Offset != targetOffset {
			p.Offset = targetOffset
			z := &c.zones[j]
			z.converged = false
			z.verifying = false
			z.probeActive = false
			z.stableCount = 0
			z.lastAdjustAt = now
			changed = true
			if c.logger != nil {
				c.logger.Debug("冷端传播: %d°C offset→%d (跟随 %d°C 的 %d RPM 上限)",
					p.Temperature, targetOffset, fanCurve[changedIdx].Temperature, effectAtChanged)
			}
		}
	}

	return changed
}

// spreadToLowerNeighbor 将上升侧的加速信号蔓延到低温侧邻居，保证曲线平滑。
// 返回 true 表示邻居偏移发生了变化。
func (c *Controller) spreadToLowerNeighbor(zoneIdx int, fanCurve []types.FanCurvePoint, minRPM, maxRPM int, now time.Time) bool {
	if zoneIdx == 0 {
		return false
	}
	neighbor := &fanCurve[zoneIdx-1]
	nz := &c.zones[zoneIdx-1]

	if nz.converged || nz.verifying {
		return false
	}
	if !nz.lastAdjustAt.IsZero() && now.Sub(nz.lastAdjustAt) < c.config.AdjustCooldown {
		return false
	}

	old := neighbor.Offset
	c.adjustZoneOffset(neighbor, c.config.Step, minRPM, maxRPM)
	if neighbor.Offset != old {
		nz.stableCount = 0
		nz.probeActive = false
		nz.lastAdjustAt = now
		if c.logger != nil {
			c.logger.Debug("邻居蔓延: %d°C offset %d→%d",
				neighbor.Temperature, old, neighbor.Offset)
		}
		return true
	}
	return false
}

// handleSpike 处理温度突变，唤醒途经的所有区间
func (c *Controller) handleSpike(currentTemp, tempDelta int, fanCurve []types.FanCurvePoint) {
	prevTemp := currentTemp - tempDelta
	lo := c.findZoneIndex(prevTemp, fanCurve)
	hi := c.findZoneIndex(currentTemp, fanCurve)
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo > 0 {
		lo--
	}
	if hi < len(c.zones)-1 {
		hi++
	}
	for i := lo; i <= hi; i++ {
		z := &c.zones[i]
		z.converged = false
		z.verifying = false
		z.probeActive = false
		z.stableCount = 0
		z.driftCount = 0
		z.trendUpCount = 0
		z.trendDownCount = 0
	}
	if c.logger != nil {
		c.logger.Debug("温度突变 %d°C→%d°C, 唤醒区间 %d°C-%d°C",
			prevTemp, currentTemp, fanCurve[lo].Temperature, fanCurve[hi].Temperature)
	}
	c.tempHistory = []tempSample{{temp: prevTemp}}
}

// Reset 重置运行状态，不清除偏移值
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

// GetZoneStatus 获取所有区间的状态
func (c *Controller) GetZoneStatus() []ZoneInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]ZoneInfo, len(c.zones))
	for i, z := range c.zones {
		result[i] = ZoneInfo{
			Converged: z.converged,
			Verifying: z.verifying,
		}
	}
	return result
}

// ZoneInfo 区间状态
type ZoneInfo struct {
	Converged bool `json:"converged"`
	Verifying bool `json:"verifying"`
}

func (c *Controller) ensureZones(fanCurve []types.FanCurvePoint) {
	n := len(fanCurve)
	if len(c.zones) == n {
		return
	}
	if c.logger != nil && len(c.zones) > 0 {
		c.logger.Debug("曲线节点数变更 %d→%d, 重置状态", len(c.zones), n)
	}
	c.zones = make([]zoneState, n)
}

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

// adjustZoneOffset 调整偏移量
func (c *Controller) adjustZoneOffset(point *types.FanCurvePoint, delta, minRPM, maxRPM int) {
	point.Offset += delta

	// 对齐到步长整数倍
	if c.config.Step > 0 {
		point.Offset = int(math.Round(float64(point.Offset)/float64(c.config.Step))) * c.config.Step
	}

	// 物理边界约束（使用 Ceil/Floor 处理负数情况）
	finalRPM := point.RPM + point.Offset
	if finalRPM < minRPM {
		// 最大允许负偏移 = ceil((minRPM - RPM) / Step) * Step
		point.Offset = int(math.Ceil(float64(minRPM-point.RPM)/float64(c.config.Step))) * c.config.Step
	} else if finalRPM > maxRPM {
		// 最大允许正偏移 = floor((maxRPM - RPM) / Step) * Step
		point.Offset = int(math.Floor(float64(maxRPM-point.RPM)/float64(c.config.Step))) * c.config.Step
	}
}

func (c *Controller) calculateTrend() int {
	if len(c.tempHistory) < 2 {
		return 0
	}
	return c.tempHistory[len(c.tempHistory)-1].temp - c.tempHistory[0].temp
}

func iabs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
