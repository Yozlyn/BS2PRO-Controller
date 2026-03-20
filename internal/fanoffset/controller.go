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

	tempRing   []tempSample
	ringHead   int
	ringCount  int
	windowSize int

	zones             []zoneState
	lastCurveTemps    []int
	lastTemp          int
	lastTempChangedAt time.Time

	config Config
	logger types.Logger
}

type tempSample struct {
	temp int
}

// zoneState 每个温度控制点的独立运行状态
type zoneState struct {
	lastAdjustAt time.Time

	stableCount int

	probeActive bool
	probeOffset int
	probeUp     bool

	verifying      bool
	verifyStartAt  time.Time
	verifyTempHigh int
	verifyTempLow  int

	converged   bool
	convergeTmp int
	driftCount  int

	trendUpCount   int
	trendDownCount int
}

// Config 控制器配置
type Config struct {
	StableDelta            int
	SpikeThreshold         int
	Step                   int
	WindowSize             int
	ConvergeCount          int
	VerifyDuration         time.Duration
	VerifyMaxDelta         int
	UpwardDriftThreshold   int
	DownwardDriftThreshold int
	DriftCount             int
	AdjustCooldown         time.Duration

	// 趋势与温度防抖配置
	HighTempTrendConfirm   int
	HighTempThreshold      int
	HighTempBoostThreshold int

	// 弹性基准线与高温滞留检测
	// 安全基准线以 maxRPM 为锚点，随温度升高线性收紧。
	// CriticalTemp: 极限温度，达到此温度不允许任何负偏移，安全下限强制为原始 RPM (default 90)
	// SafeTemp: 安全温度，低于此温度安全下限仅为硬件最低转速，允许最大幅度降噪 (default 60)
	// StagnationDuration: 高温下温度长期不变时，主动触发一次升速的等待时间 (default 15s)
	CriticalTemp       int
	SafeTemp           int
	StagnationDuration time.Duration
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
		HighTempThreshold:      75,
		HighTempBoostThreshold: 85,
		CriticalTemp:           90,
		SafeTemp:               60,
		StagnationDuration:     15 * time.Second,
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
		cfg.HighTempThreshold = 75
	}
	if cfg.HighTempBoostThreshold <= 0 {
		cfg.HighTempBoostThreshold = 85
	}
	if cfg.CriticalTemp <= cfg.SafeTemp {
		cfg.CriticalTemp = 90
		cfg.SafeTemp = 60
	}
	if cfg.StagnationDuration <= 0 {
		cfg.StagnationDuration = 15 * time.Second
	}

	return &Controller{
		config:            cfg,
		windowSize:        cfg.WindowSize,
		tempRing:          make([]tempSample, cfg.WindowSize),
		ringHead:          0,
		ringCount:         0,
		lastTemp:          -1,
		lastTempChangedAt: time.Now(),
		logger:            logger,
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

	// 记录温度真实变化时间，处理突发尖峰
	if c.lastTemp >= 0 {
		if currentTemp != c.lastTemp {
			c.lastTempChangedAt = now
			if d := currentTemp - c.lastTemp; iabs(d) >= c.config.SpikeThreshold {
				c.handleSpike(currentTemp, d, fanCurve)
			}
		}
	} else {
		c.lastTempChangedAt = now
	}
	c.lastTemp = currentTemp

	// 维护温度历史窗口
	c.tempRing[c.ringHead] = tempSample{temp: currentTemp}
	c.ringHead = (c.ringHead + 1) % c.windowSize
	if c.ringCount < c.windowSize {
		c.ringCount++
	}
	if c.ringCount < 2 {
		return false
	}

	zoneIdx := c.findZoneIndex(currentTemp, fanCurve)
	zone := &c.zones[zoneIdx]
	point := &fanCurve[zoneIdx]

	// 弹性安全基准线
	minSafeRPM := c.getMinSafeRPM(currentTemp, minRPM, maxRPM)
	minSafeOffset := minSafeRPM - point.RPM
	// 极限温度以上不允许任何负偏移（降噪）
	if currentTemp >= c.config.CriticalTemp {
		minSafeOffset = 0
	}

	defenseTriggered := false
	effRPM := point.RPM + point.Offset

	// 物理安全防线：实际转速跌破弹性下限时强制拉回
	if effRPM < minSafeRPM {
		oldOffset := point.Offset
		point.Offset = minSafeRPM - point.RPM
		zone.stableCount = 0
		zone.verifying = false
		zone.converged = false
		zone.lastAdjustAt = now
		defenseTriggered = true
		if c.logger != nil {
			c.logger.Info("[安全防线] %d°C 触发安全防线, 强制重置转速: offset %d->%d", point.Temperature, oldOffset, point.Offset)
		}
	}

	oldOffset := point.Offset
	isHighTemp := currentTemp >= c.config.HighTempThreshold

	// 高温滞留检测：在高温区若温度长期没有真实变化
	stagnationHandled := false
	if !defenseTriggered && !zone.converged && !zone.verifying && !zone.probeActive &&
		currentTemp >= c.config.HighTempThreshold &&
		(zone.lastAdjustAt.IsZero() || now.Sub(zone.lastAdjustAt) >= c.config.AdjustCooldown) &&
		now.Sub(c.lastTempChangedAt) >= c.config.StagnationDuration {

		c.lastTempChangedAt = now // 重置滞留计时，避免连续触发
		c.adjustZoneOffset(point, c.config.Step, minRPM, maxRPM)
		zone.stableCount = 0
		zone.trendUpCount = 0
		zone.trendDownCount = 0
		zone.lastAdjustAt = now
		stagnationHandled = true
		if c.logger != nil {
			c.logger.Info("[滞留检测] %d°C 高温滞留 >= %v, 主动升速: offset %d->%d",
				point.Temperature, c.config.StagnationDuration, oldOffset, point.Offset)
		}
	}

NORMAL_LOGIC:
	switch {
	case !defenseTriggered && !stagnationHandled:
		if zone.converged {
			c.checkDrift(zone, currentTemp, point)
			break NORMAL_LOGIC
		}

		trend := c.calculateTrend()

		// 处于验证期
		if zone.verifying {
			c.handleVerifying(zoneIdx, fanCurve, currentTemp, trend, now, minRPM, maxRPM)
			break NORMAL_LOGIC
		}

		// 处理试探的回调
		if zone.probeActive {
			zone.probeActive = false
			if !zone.probeUp {
				probeResponseThreshold := c.config.StableDelta
				if zoneIdx == len(fanCurve)-1 {
					probeResponseThreshold = c.config.StableDelta * 2
				}
				if trend >= probeResponseThreshold {
					oldOff := point.Offset
					point.Offset = zone.probeOffset
					zone.lastAdjustAt = now
					c.startVerifying(zone, currentTemp, now)
					if c.logger != nil {
						c.logger.Info("[试探回退] 区间 %d°C 降噪试探中止(trend=%d): offset回退 %d->%d",
							point.Temperature, trend, oldOff, point.Offset)
					}
					break NORMAL_LOGIC
				}
				zone.stableCount = 0
			}
		}

		if !zone.lastAdjustAt.IsZero() && now.Sub(zone.lastAdjustAt) < c.config.AdjustCooldown {
			break NORMAL_LOGIC
		}

		switch {
		case trend >= c.config.StableDelta:
			// 升温处理逻辑
			zone.trendDownCount = 0
			zone.trendUpCount++

			requiredConfirm := c.config.HighTempTrendConfirm
			trendMultiplier := float64(trend) / float64(c.config.StableDelta)

			if currentTemp >= c.config.CriticalTemp {
				requiredConfirm = 1
			} else if currentTemp >= c.config.HighTempBoostThreshold {
				if trendMultiplier <= 2.0 {
					requiredConfirm *= 3
				}
			} else if isHighTemp {
				if trend <= 5 {
					requiredConfirm *= 5
				} else {
					requiredConfirm *= 2
				}
			} else {
				if trendMultiplier <= 1.5 {
					requiredConfirm *= 4
				} else if trendMultiplier <= 2.5 {
					requiredConfirm *= 2
				}
			}

			if zone.trendUpCount < requiredConfirm {
				if c.logger != nil {
					c.logger.Info("[升温防抖] %d°C 升温防抖(trend=%d): %d/%d",
						point.Temperature, trend, zone.trendUpCount, requiredConfirm)
				}
				break NORMAL_LOGIC
			}

			zone.trendUpCount = 0
			upStep := c.config.Step
			if currentTemp >= c.config.CriticalTemp {
				upStep = c.config.Step * 3
			} else if currentTemp >= c.config.HighTempBoostThreshold {
				upStep = c.config.Step * 2
			}

			c.adjustZoneOffset(point, upStep, minRPM, maxRPM)
			zone.stableCount = 0

			if c.logger != nil {
				c.logger.Info("[确认升温] %d°C 确认升温: offset %d->%d", point.Temperature, oldOffset, point.Offset)
			}

		case trend <= -c.config.StableDelta:
			// 降温处理逻辑
			if isHighTemp {
				zone.trendDownCount++
				zone.trendUpCount = 0
				if zone.trendDownCount < c.config.HighTempTrendConfirm {
					break NORMAL_LOGIC
				}
				zone.trendDownCount = 0
			}

			dropStep := c.config.Step * 3
			dropReason := "标准降温(3x)"
			trendMultiplier := float64(-trend) / float64(c.config.StableDelta)

			if trendMultiplier >= 3.0 {
				dropStep = c.config.Step * 6
				dropReason = "温度暴降(6x)"
			} else if trendMultiplier >= 1.5 {
				dropStep = c.config.Step * 4
				dropReason = "快速降温(4x)"
			}

			if point.Offset > 0 {
				dropStep += c.config.Step * 2
				dropReason += "+快速消除正偏移"
			}

			if point.Offset-dropStep < minSafeOffset {
				if point.Offset > minSafeOffset {
					dropStep = point.Offset - minSafeOffset
					dropReason += "(触底拦截)"
				} else {
					dropStep = 0
				}
			}

			if dropStep > 0 {
				c.adjustZoneOffset(point, -dropStep, minRPM, maxRPM)
				zone.stableCount = 0
				if c.logger != nil {
					c.logger.Info("[确认降温] %d°C 降温(%s): offset %d->%d",
						point.Temperature, dropReason, oldOffset, point.Offset)
				}
			}

			// 降温时，迅速消除下游高温区因防线触发产生的正偏移包袱
			for i := zoneIdx + 1; i < len(fanCurve); i++ {
				p := &fanCurve[i]
				if p.Offset > 0 {
					p.Offset -= c.config.Step * 2
					if p.Offset < 0 {
						p.Offset = 0
					}
					c.zones[i].converged = false
					c.zones[i].verifying = false
				}

				if p.RPM+p.Offset > point.RPM+point.Offset {
					c.adjustZoneOffset(p, -c.config.Step, minRPM, maxRPM)
					targetRPM := p.RPM + point.Offset
					if p.RPM+p.Offset < targetRPM {
						c.setZoneEffectiveRPM(p, targetRPM, minRPM, maxRPM)
					}
					// 保证下游节点不跌破其自身的弹性基准线
					localMinSafeOffset := c.getMinSafeRPM(p.Temperature, minRPM, maxRPM) - p.RPM
					if p.Offset < localMinSafeOffset {
						p.Offset = localMinSafeOffset
					}
					c.zones[i].converged = false
					c.zones[i].verifying = false
				}
			}

		default:
			// 稳定状态处理
			zone.trendUpCount = 0
			zone.trendDownCount = 0
			zone.stableCount++

			if zone.stableCount >= c.config.ConvergeCount {
				zone.stableCount = 0
				zone.probeOffset = point.Offset

				if currentTemp >= c.config.HighTempBoostThreshold {
					c.startVerifying(zone, currentTemp, now)
					break NORMAL_LOGIC
				}

				if point.Offset < minSafeOffset {
					rebound := c.config.Step * 3
					if point.Offset+rebound > minSafeOffset {
						rebound = minSafeOffset - point.Offset
					}
					c.adjustZoneOffset(point, rebound, minRPM, maxRPM)
					if c.logger != nil {
						c.logger.Info("[回弹] %d°C 柔和回弹至基准线: offset->%d", point.Temperature, point.Offset)
					}
					break NORMAL_LOGIC
				}

				zone.probeUp = false
				probeDrop := c.config.Step

				if point.Offset > 0 {
					probeDrop = c.config.Step * 3
				} else if point.Offset > -c.config.Step*5 {
					probeDrop = c.config.Step * 2
				} else if point.Offset <= minSafeOffset {
					c.startVerifying(zone, currentTemp, now)
					break NORMAL_LOGIC
				}

				// 高温区试探更保守，避免正偏移一次性探到负偏移后被安全防线反复打回
				if isHighTemp {
					// 高温下不允许一次试探跨过 0，最多回到 0
					if point.Offset > 0 && point.Offset-probeDrop < 0 {
						probeDrop = point.Offset
					}

					// 与安全下限保持至少 1 个 step 的缓冲，降低贴线抖动
					minProbeOffset := minSafeOffset + c.config.Step
					if point.Offset-probeDrop < minProbeOffset {
						probeDrop = point.Offset - minProbeOffset
					}

					if probeDrop <= 0 {
						c.startVerifying(zone, currentTemp, now)
						break NORMAL_LOGIC
					}
				}

				c.adjustZoneOffset(point, -probeDrop, minRPM, maxRPM)

				if point.Offset == zone.probeOffset {
					c.startVerifying(zone, currentTemp, now)
				} else {
					zone.probeActive = true
					if c.logger != nil {
						c.logger.Info("[试探下调] %d°C 温度稳定, 试探下调: offset %d->%d",
							point.Temperature, zone.probeOffset, point.Offset)
					}
				}
			}
		}
	}

	// 维持全局单调性约束
	anchorChanged := c.enforceMonotonicityFromActive(zoneIdx, fanCurve, minRPM, maxRPM)

	if point.Offset != oldOffset || anchorChanged {
		if point.Offset != oldOffset && !defenseTriggered {
			zone.lastAdjustAt = now
		}
		return true
	}
	return false
}

// enforceMonotonicityFromActive 维持左侧转速抚平与右侧转速单调递增
func (c *Controller) enforceMonotonicityFromActive(activeIdx int, fanCurve []types.FanCurvePoint, minRPM, maxRPM int) bool {
	changed := false

	for i := activeIdx - 1; i >= 0; i-- {
		p := &fanCurve[i]
		effRPM := p.RPM + p.Offset
		rightP := &fanCurve[i+1]
		rightRPM := rightP.RPM + rightP.Offset

		maxAllowedRPM := rightRPM - c.config.Step
		if effRPM > maxAllowedRPM {
			oldOffset := p.Offset
			c.setZoneEffectiveRPM(p, maxAllowedRPM, minRPM, maxRPM)
			if p.Offset != oldOffset {
				changed = true
			}
		}
	}

	for i := activeIdx + 1; i < len(fanCurve); i++ {
		p := &fanCurve[i]
		leftP := &fanCurve[i-1]
		oldOffset := p.Offset

		if p.Offset > leftP.Offset {
			targetOffset := leftP.Offset
			localMinSafeRPM := c.getMinSafeRPM(p.Temperature, minRPM, maxRPM)
			localMinSafeOffset := localMinSafeRPM - p.RPM

			if targetOffset < localMinSafeOffset {
				targetOffset = localMinSafeOffset
			}
			p.Offset = targetOffset
			c.setZoneEffectiveRPM(p, p.RPM+p.Offset, minRPM, maxRPM)
		}

		effRPM := p.RPM + p.Offset
		leftRPM := leftP.RPM + leftP.Offset

		minRequiredRPM := leftRPM + c.config.Step
		if effRPM < minRequiredRPM {
			c.setZoneEffectiveRPM(p, minRequiredRPM, minRPM, maxRPM)
		}

		if p.Offset != oldOffset {
			changed = true
			if i < len(c.zones) {
				c.zones[i].converged = false
				c.zones[i].verifying = false
			}
		}
	}

	lastIdx := len(fanCurve) - 1
	if lastIdx >= 0 {
		p := &fanCurve[lastIdx]
		if p.Temperature >= c.config.CriticalTemp && (p.RPM+p.Offset) < maxRPM {
			oldOffset := p.Offset
			c.setZoneEffectiveRPM(p, maxRPM, minRPM, maxRPM)
			if p.Offset != oldOffset {
				changed = true
			}
			for i := lastIdx - 1; i >= 0; i-- {
				lp := &fanCurve[i]
				rp := &fanCurve[i+1]
				maxAllowed := (rp.RPM + rp.Offset) - c.config.Step
				if lp.RPM+lp.Offset > maxAllowed {
					c.setZoneEffectiveRPM(lp, maxAllowed, minRPM, maxRPM)
				}
			}
		}
	}

	return changed
}

func (c *Controller) setZoneEffectiveRPM(point *types.FanCurvePoint, targetRPM, minRPM, maxRPM int) {
	c.adjustZoneOffset(point, targetRPM-(point.RPM+point.Offset), minRPM, maxRPM)
}

func (c *Controller) handleVerifying(zoneIdx int, fanCurve []types.FanCurvePoint, currentTemp, trend int, now time.Time, minRPM, maxRPM int) {
	zone := &c.zones[zoneIdx]
	point := &fanCurve[zoneIdx]

	if currentTemp > zone.verifyTempHigh {
		zone.verifyTempHigh = currentTemp
	}
	if currentTemp < zone.verifyTempLow {
		zone.verifyTempLow = currentTemp
	}

	breakThreshold := c.config.StableDelta * 2
	if currentTemp >= c.config.CriticalTemp {
		breakThreshold = 1
	} else if currentTemp >= c.config.HighTempBoostThreshold {
		breakThreshold = 3
	}

	if trend >= breakThreshold {
		zone.verifying = false
		zone.stableCount = 0
		oldOffset := point.Offset
		step := c.config.Step
		if currentTemp >= c.config.HighTempBoostThreshold {
			step = c.config.Step * 2
		}
		c.adjustZoneOffset(point, step, minRPM, maxRPM)
		zone.lastAdjustAt = now
		if c.logger != nil {
			c.logger.Info("[验证中断] %d°C 验证期因升温打断(trend=%d): offset %d->%d",
				point.Temperature, trend, oldOffset, point.Offset)
		}
		return
	}

	if zone.verifyTempHigh-zone.verifyTempLow > c.config.VerifyMaxDelta {
		zone.verifying = false
		zone.stableCount = 0
		return
	}

	if now.Sub(zone.verifyStartAt) < c.config.VerifyDuration {
		return
	}

	zone.verifying = false
	zone.converged = true
	zone.convergeTmp = currentTemp
	zone.stableCount = 0
	zone.driftCount = 0
	if c.logger != nil {
		c.logger.Info("[收敛] %d°C 成功收敛, 维持偏移 offset=%d", point.Temperature, point.Offset)
	}
}

func (c *Controller) startVerifying(zone *zoneState, currentTemp int, now time.Time) {
	zone.verifying = true
	zone.verifyStartAt = now
	zone.verifyTempHigh = currentTemp
	zone.verifyTempLow = currentTemp
}

func (c *Controller) checkDrift(zone *zoneState, currentTemp int, point *types.FanCurvePoint) {
	upwardThreshold := c.config.UpwardDriftThreshold
	downwardThreshold := c.config.DownwardDriftThreshold
	requiredCount := c.config.DriftCount

	if point.Temperature < c.config.HighTempThreshold {
		upwardThreshold += c.config.StableDelta
		downwardThreshold -= c.config.StableDelta * 2
	}

	delta := currentTemp - zone.convergeTmp
	if delta >= upwardThreshold || delta <= downwardThreshold {
		zone.driftCount++
		if zone.driftCount >= requiredCount {
			zone.converged = false
			zone.stableCount = 0
			zone.driftCount = 0
			zone.verifying = false
			if c.logger != nil {
				c.logger.Info("[漂移重置] %d°C 检测到温度漂移, 解除收敛状态", point.Temperature)
			}
		}
	} else {
		zone.driftCount = 0
	}
}

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
		c.logger.Info("[瞬变检测] 捕捉到温度瞬变 %d°C -> %d°C, 重置区间状态", prevTemp, currentTemp)
	}
	c.tempRing[0] = tempSample{temp: prevTemp}
	c.ringHead = 1 % c.windowSize
	c.ringCount = 1
}

func (c *Controller) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ringHead = 0
	c.ringCount = 0
	c.lastTemp = -1
	c.lastTempChangedAt = time.Now()
	c.lastCurveTemps = c.lastCurveTemps[:0]
	for i := range c.zones {
		c.zones[i] = zoneState{}
	}
}

func (c *Controller) ResetOffsets(fanCurve []types.FanCurvePoint) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ringHead = 0
	c.ringCount = 0
	c.lastTemp = -1
	c.lastTempChangedAt = time.Now()
	c.lastCurveTemps = c.lastCurveTemps[:0]
	for i := range c.zones {
		c.zones[i] = zoneState{}
	}
	for i := range fanCurve {
		fanCurve[i].Offset = 0
	}
}

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

type ZoneInfo struct {
	Converged bool `json:"converged"`
	Verifying bool `json:"verifying"`
}

// GetCurrentZoneState 返回当前温度所在区间的偏移引擎状态（补偿中/回收中/稳定）
func (c *Controller) GetCurrentZoneState(temp int, fanCurve []types.FanCurvePoint) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(fanCurve) == 0 {
		return "稳定"
	}
	c.ensureZones(fanCurve)
	idx := c.findZoneIndex(temp, fanCurve)
	if idx < 0 || idx >= len(fanCurve) || idx >= len(c.zones) {
		return "稳定"
	}
	zone := c.zones[idx]
	offset := fanCurve[idx].Offset
	if zone.converged {
		return "稳定"
	}
	if offset > 0 {
		return "补偿中"
	}
	if offset < 0 {
		return "回收中"
	}
	return "稳定"
}

// ensureZones 按温度点匹配继承已有zoneState
func (c *Controller) ensureZones(fanCurve []types.FanCurvePoint) {
	n := len(fanCurve)
	if len(c.zones) == n {
		changed := false
		for i, p := range fanCurve {
			if i >= len(c.lastCurveTemps) || c.lastCurveTemps[i] != p.Temperature {
				changed = true
				break
			}
		}
		if !changed {
			return
		}
		// 温度点被编辑，同样走继承逻辑
	}

	oldByTemp := make(map[int]zoneState, len(c.lastCurveTemps))
	for i, t := range c.lastCurveTemps {
		if i < len(c.zones) {
			oldByTemp[t] = c.zones[i]
		}
	}

	newZones := make([]zoneState, n)
	inherited, reset := 0, 0
	for i, p := range fanCurve {
		if old, ok := oldByTemp[p.Temperature]; ok {
			newZones[i] = old
			inherited++
		} else {
			reset++
		}
	}

	if c.logger != nil && (len(c.zones) != n || reset > 0) {
		c.logger.Info("[确保区域] 曲线节点变更: %d -> %d 节点, 继承 %d 个区间状态, 重置 %d 个",
			len(c.zones), n, inherited, reset)
	}

	c.zones = newZones

	// 刷新温度缓存
	c.lastCurveTemps = make([]int, n)
	for i, p := range fanCurve {
		c.lastCurveTemps[i] = p.Temperature
	}
}
func (c *Controller) findZoneIndex(temp int, fanCurve []types.FanCurvePoint) int {
	if len(fanCurve) == 0 {
		return 0
	}
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

func (c *Controller) adjustZoneOffset(point *types.FanCurvePoint, delta, minRPM, maxRPM int) {
	point.Offset += delta

	if c.config.Step > 0 {
		point.Offset = int(math.Round(float64(point.Offset)/float64(c.config.Step))) * c.config.Step
	}

	finalRPM := point.RPM + point.Offset
	if finalRPM < minRPM {
		point.Offset = int(math.Ceil(float64(minRPM-point.RPM)/float64(c.config.Step))) * c.config.Step
	} else if finalRPM > maxRPM {
		point.Offset = int(math.Floor(float64(maxRPM-point.RPM)/float64(c.config.Step))) * c.config.Step
	}
}

// getMinSafeRPM 计算当前温度下的安全转速下限
func (c *Controller) getMinSafeRPM(temp, minRPM, maxRPM int) int {
	if temp <= c.config.SafeTemp {
		return minRPM
	}

	safeRPM := minRPM
	midThreshold := (c.config.HighTempThreshold + c.config.HighTempBoostThreshold) / 2

	switch {
	case temp >= 100:
		safeRPM = maxRPM
	case temp >= c.config.CriticalTemp: // default 90
		safeRPM = int(float64(maxRPM) * 0.8)
	case temp >= c.config.HighTempBoostThreshold: // default 85
		safeRPM = int(float64(maxRPM) * 0.6)
	case temp >= midThreshold: // default 80
		safeRPM = int(float64(maxRPM) * 0.5)
	case temp >= c.config.HighTempThreshold: // default 75
		safeRPM = int(float64(maxRPM) * 0.4)
	}

	if safeRPM < minRPM {
		safeRPM = minRPM
	}
	return safeRPM
}

// calculateTrend 基于温度历史窗口计算趋势。
func (c *Controller) calculateTrend() int {
	if c.ringCount < 2 {
		return 0
	}
	oldestIdx := (c.ringHead + c.windowSize - c.ringCount) % c.windowSize
	newestIdx := (c.ringHead - 1 + c.windowSize) % c.windowSize
	return c.tempRing[newestIdx].temp - c.tempRing[oldestIdx].temp
}

func iabs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
