// Package fanoffset 提供自动风扇曲线偏移控制器
package fanoffset

import (
	"math"
	"strconv"
	"strings"
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

	zones              []zoneState
	learnedMemory      *learnedMemoryStore
	persistence        *learnedMemoryPersistence
	lastCurveTemps     []int
	lastCurveSignature string
	lastTemp           int
	lastTempChangedAt  time.Time
	emaTemp            float64
	emaSlowTemp        float64
	emaTrend           float64
	emaInitialized     bool
	pendingSpikeBase   int
	pendingSpikeDelta  int
	pendingSpike       bool

	config Config
	logger types.Logger

	activeZoneTemp  int
	activeZoneValid bool
}

type tempSample struct {
	temp int
}

type learnedState struct {
	offset     float64
	confidence float64
	successes  int
	failures   int
	hasSeed    bool
}

type learnedMemoryKey struct {
	curveSignature string
	zoneTemp       int
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
	verifyScoreSum float64
	verifySamples  int

	converged   bool
	convergeTmp int
	driftCount  int

	trendUpCount   int
	trendDownCount int

	learnedOffset       float64
	learnedConfidence   float64
	learnSuccesses      int
	learnFailures       int
	memorySeedPending   bool
	memoryCooldownUntil time.Time
	memoryFailStreak    int
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
	TempEMAAlpha           float64
	TempEMASlowAlpha       float64
	TrendEMAAlpha          float64
	PreheatBand            int
	PreheatClampStep       int
	LearnRate              float64
	LearnConfidenceGain    float64
	LearnConfidenceDecay   float64

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
		TempEMAAlpha:           0.35,
		TempEMASlowAlpha:       0.12,
		TrendEMAAlpha:          0.45,
		PreheatBand:            5,
		PreheatClampStep:       100,
		LearnRate:              0.18,
		LearnConfidenceGain:    0.12,
		LearnConfidenceDecay:   0.08,
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
	if cfg.TempEMAAlpha <= 0 || cfg.TempEMAAlpha >= 1 {
		cfg.TempEMAAlpha = 0.35
	}
	if cfg.TempEMASlowAlpha <= 0 || cfg.TempEMASlowAlpha >= cfg.TempEMAAlpha {
		cfg.TempEMASlowAlpha = 0.12
	}
	if cfg.TrendEMAAlpha <= 0 || cfg.TrendEMAAlpha >= 1 {
		cfg.TrendEMAAlpha = 0.45
	}
	if cfg.PreheatBand <= 0 {
		cfg.PreheatBand = 5
	}
	if cfg.PreheatClampStep <= 0 {
		cfg.PreheatClampStep = cfg.Step
	}
	if cfg.LearnRate <= 0 || cfg.LearnRate >= 1 {
		cfg.LearnRate = 0.18
	}
	if cfg.LearnConfidenceGain <= 0 || cfg.LearnConfidenceGain >= 1 {
		cfg.LearnConfidenceGain = 0.12
	}
	if cfg.LearnConfidenceDecay <= 0 || cfg.LearnConfidenceDecay >= 1 {
		cfg.LearnConfidenceDecay = 0.08
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
		learnedMemory:     newLearnedMemoryStore(),
		ringHead:          0,
		ringCount:         0,
		lastTemp:          -1,
		lastTempChangedAt: time.Now(),
		logger:            logger,
	}
}

// EnablePersistence 启用长期记忆 journal/snapshot 持久化。
func (c *Controller) EnablePersistence(baseDir string) error {
	persistence, err := newLearnedMemoryPersistence(baseDir, c.logger)
	if err != nil {
		return err
	}
	store, loadErr := persistence.Load()
	if loadErr != nil {
		if c.logger != nil {
			c.logger.Warn("风扇偏移长期记忆加载失败，已回落为空状态", "error", loadErr)
		}
		store = newLearnedMemoryStore()
	}

	c.mu.Lock()
	c.persistence = persistence
	if store != nil {
		c.learnedMemory = store
	}
	loaded := 0
	if c.learnedMemory != nil {
		loaded = c.learnedMemory.Len()
	}
	c.mu.Unlock()

	if loaded > 0 && c.logger != nil {
		c.logger.Info("风扇偏移长期记忆加载完成", "entries", loaded, "path", baseDir)
	}
	return nil
}

// FlushPersistence 将长期记忆主动压成 snapshot。
func (c *Controller) FlushPersistence() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.flushPersistenceLocked("manual")
}

// Close 关闭控制器前刷新持久化状态。
func (c *Controller) Close() error {
	return c.FlushPersistence()
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
	skipSpikeDetect := c.processPendingSpike(currentTemp, fanCurve)
	if c.lastTemp >= 0 {
		if currentTemp != c.lastTemp {
			c.lastTempChangedAt = now
			if !skipSpikeDetect {
				if d := currentTemp - c.lastTemp; iabs(d) >= c.config.SpikeThreshold {
					c.pendingSpikeBase = c.lastTemp
					c.pendingSpikeDelta = d
					c.pendingSpike = true
					if c.logger != nil {
						c.logger.Debug("风扇偏移捕捉到候选瞬变", "state", "pending", "prev_temp", c.lastTemp, "current_temp", currentTemp, "delta", d)
					}
				}
			}
		}
	} else {
		c.lastTempChangedAt = now
	}
	c.lastTemp = currentTemp
	c.updateEMA(currentTemp)

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
	c.noteZoneEntry(zone, point.Temperature)

	// 弹性安全基准线
	minSafeRPM := c.getMinSafeRPM(currentTemp, minRPM, maxRPM)
	minSafeOffset := minSafeRPM - point.RPM
	minSafeOffset = c.applyPreheatClamp(minSafeOffset, currentTemp)
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
			c.logger.Info("风扇偏移触发安全防线",
				"zone_temp", point.Temperature,
				"old_offset", oldOffset,
				"new_offset", point.Offset)
		}
	}

	oldOffset := point.Offset
	isHighTemp := currentTemp >= c.config.HighTempThreshold

	// 高温滞留检测：在高温区若温度长期没有真实变化
	stagnationHandled := false
	stagnationPressure := c.hasHighTempStagnationPressure(zone, currentTemp)
	if !defenseTriggered && !zone.converged && !zone.verifying && !zone.probeActive &&
		currentTemp >= c.config.HighTempThreshold &&
		stagnationPressure &&
		(zone.lastAdjustAt.IsZero() || now.Sub(zone.lastAdjustAt) >= c.config.AdjustCooldown) &&
		now.Sub(c.lastTempChangedAt) >= c.config.StagnationDuration {

		c.lastTempChangedAt = now // 重置滞留计时，避免连续触发
		c.adjustZoneOffset(point, c.config.Step, minRPM, maxRPM)
		if point.Offset != oldOffset {
			zone.stableCount = 0
			zone.trendUpCount = 0
			zone.trendDownCount = 0
			zone.lastAdjustAt = now
			stagnationHandled = true
		}
		if c.logger != nil && point.Offset != oldOffset {
			c.logger.Info("风扇偏移高温滞留主动升速",
				"zone_temp", point.Temperature,
				"stagnation", c.config.StagnationDuration,
				"old_offset", oldOffset,
				"new_offset", point.Offset)
		}
	}

NORMAL_LOGIC:
	switch {
	case !defenseTriggered && !stagnationHandled:
		if zone.converged {
			c.checkDrift(zone, currentTemp, point)
			break NORMAL_LOGIC
		}

		trend := c.getControlTrend()

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
						c.logger.Info("风扇偏移试探回退",
							"zone_temp", point.Temperature,
							"trend", trend,
							"old_offset", oldOff,
							"new_offset", point.Offset,
							"reason", "probe_cooling_aborted")
					}
					break NORMAL_LOGIC
				}
				zone.stableCount = 0
			} else {
				zone.lastAdjustAt = now
				c.startVerifying(zone, currentTemp, now)
				if c.logger != nil {
					c.logger.Info("风扇偏移试探上调进入验证",
						"zone_temp", point.Temperature,
						"offset", point.Offset,
						"trend", trend,
						"reason", "probe_heating_verify")
				}
				break NORMAL_LOGIC
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

			requiredConfirm := c.getRequiredTrendConfirm(currentTemp, trend)

			if zone.trendUpCount < requiredConfirm {
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
				c.logger.Debug("风扇偏移确认升温",
					"zone_temp", point.Temperature,
					"old_offset", oldOffset,
					"new_offset", point.Offset,
					"reason", c.describeUpReason(currentTemp, trend, requiredConfirm),
					"raw_temp", currentTemp,
					"ema_fast", c.emaTemp,
					"ema_slow", c.emaSlowTemp,
					"ema_trend", c.emaTrend,
					"trend", trend,
					"confirm", requiredConfirm)
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
					c.logger.Debug("风扇偏移确认降温",
						"zone_temp", point.Temperature,
						"reason", dropReason,
						"old_offset", oldOffset,
						"new_offset", point.Offset,
						"raw_temp", currentTemp,
						"ema_fast", c.emaTemp,
						"ema_slow", c.emaSlowTemp,
						"ema_trend", c.emaTrend,
						"trend", trend,
						"min_safe_offset", minSafeOffset)
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
			stabilityCount := zone.stableCount
			zone.trendUpCount = 0
			zone.trendDownCount = 0
			zone.stableCount++

			if zone.stableCount >= c.config.ConvergeCount {
				zone.stableCount = 0
				zone.probeOffset = point.Offset
				if zone.memorySeedPending {
					if c.applyLearnedOffsetMemory(zone, point, currentTemp, trend, minSafeOffset, now, minRPM, maxRPM, true) {
						break NORMAL_LOGIC
					}
					zone.memorySeedPending = false
				} else if zone.learnedConfidence >= 0.55 && point.Offset > minSafeOffset && !zone.converged {
					if c.applyLearnedOffsetMemory(zone, point, currentTemp, trend, minSafeOffset, now, minRPM, maxRPM, false) {
						break NORMAL_LOGIC
					}
				}

				if c.handleHighTempProtectionSettle(zone, point, currentTemp, minSafeOffset, minRPM, maxRPM) {
					break NORMAL_LOGIC
				}

				if point.Offset < minSafeOffset {
					rebound := c.config.Step * 3
					if point.Offset+rebound > minSafeOffset {
						rebound = minSafeOffset - point.Offset
					}
					c.adjustZoneOffset(point, rebound, minRPM, maxRPM)
					if c.logger != nil {
						c.logger.Debug("风扇偏移回弹至安全基线",
							"zone_temp", point.Temperature,
							"offset", point.Offset,
							"min_safe_offset", minSafeOffset)
					}
					break NORMAL_LOGIC
				}

				if c.shouldProbeUp(point, currentTemp) {
					zone.probeUp = true
					probeUpStep := c.calculateProbeUpStep(currentTemp)
					c.adjustZoneOffset(point, probeUpStep, minRPM, maxRPM)
					if point.Offset == zone.probeOffset {
						c.startVerifying(zone, currentTemp, now)
					} else {
						zone.probeActive = true
						if c.logger != nil {
							c.logger.Debug("风扇偏移稳定区试探上调",
								"zone_temp", point.Temperature,
								"old_offset", zone.probeOffset,
								"new_offset", point.Offset,
								"reason", "stable_zone_heat_pressure")
						}
					}
					break NORMAL_LOGIC
				}

				zone.probeUp = false
				if point.Offset <= minSafeOffset {
					c.startVerifying(zone, currentTemp, now)
					break NORMAL_LOGIC
				}
				if c.handleHighTempProtectionSettle(zone, point, currentTemp, minSafeOffset, minRPM, maxRPM) {
					break NORMAL_LOGIC
				}
				probeDrop := c.calculateProbeDrop(point, currentTemp, stabilityCount, minSafeOffset)
				if probeDrop <= 0 {
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
						c.logger.Debug("风扇偏移稳定区试探下调",
							"zone_temp", point.Temperature,
							"old_offset", zone.probeOffset,
							"new_offset", point.Offset,
							"reason", "stable_zone_probe_cooling")
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

func (c *Controller) enterHighTempProtectionState(zone *zoneState, currentTemp int) {
	zone.verifying = false
	zone.probeActive = false
	zone.probeUp = false
	zone.converged = true
	zone.convergeTmp = currentTemp
	zone.stableCount = 0
	zone.driftCount = 0
	zone.verifyScoreSum = 0
	zone.verifySamples = 0
}

func (c *Controller) reboundToHighTempProtection(point *types.FanCurvePoint, currentTemp, minSafeOffset, minRPM, maxRPM int) bool {
	protectionFloor := max(minSafeOffset, 0)
	if point.Offset >= protectionFloor {
		return false
	}
	rebound := c.config.Step * 3
	if point.Offset+rebound > protectionFloor {
		rebound = protectionFloor - point.Offset
	}
	if rebound <= 0 {
		return false
	}
	c.adjustZoneOffset(point, rebound, minRPM, maxRPM)
	if c.logger != nil {
		c.logger.Debug("风扇偏移高温保护区回弹",
			"zone_temp", point.Temperature,
			"offset", point.Offset,
			"protection_floor", protectionFloor,
			"raw_temp", currentTemp)
	}
	return true
}

func (c *Controller) handleHighTempProtectionSettle(zone *zoneState, point *types.FanCurvePoint, currentTemp, minSafeOffset, minRPM, maxRPM int) bool {
	if currentTemp < c.config.HighTempBoostThreshold {
		return false
	}
	c.reboundToHighTempProtection(point, currentTemp, minSafeOffset, minRPM, maxRPM)
	c.enterHighTempProtectionState(zone, currentTemp)
	return true
}

func (c *Controller) clearLearnState(zone *zoneState, temp int) {
	curveSignature := c.storedCurveSignatureLocked()
	if c.learnedMemory != nil {
		c.learnedMemory.Delete(curveSignature, temp)
	}
	c.persistLearnedDeleteLocked(curveSignature, temp)
	zone.learnedOffset = 0
	zone.learnedConfidence = 0
	zone.learnSuccesses = 0
	zone.learnFailures = 0
	c.clearLearnedMemorySuppression(zone)
}

func (c *Controller) noteZoneEntry(zone *zoneState, zoneTemp int) {
	if c.activeZoneValid && c.activeZoneTemp == zoneTemp {
		return
	}
	for i := range c.zones {
		c.zones[i].memorySeedPending = false
	}
	zone.memorySeedPending = true
	c.activeZoneTemp = zoneTemp
	c.activeZoneValid = true
}

func (c *Controller) applyLearnedStateToZone(zone *zoneState, state learnedState) {
	zone.learnedOffset = state.offset
	zone.learnedConfidence = state.confidence
	zone.learnSuccesses = state.successes
	zone.learnFailures = state.failures
}

func (c *Controller) buildMigratedLearnedState(curveSignature string, point types.FanCurvePoint) (learnedState, learnedMemoryCandidate, bool) {
	if c.learnedMemory == nil || point.Temperature >= c.config.HighTempBoostThreshold {
		return learnedState{}, learnedMemoryCandidate{}, false
	}
	minConfidence := 0.35
	candidate, ok := c.learnedMemory.BestAlternativeForZone(curveSignature, point.Temperature, point.RPM, minConfidence)
	if !ok {
		return learnedState{}, learnedMemoryCandidate{}, false
	}
	maxRPMGap := max(c.config.Step*8, 800)
	if candidate.rpmGap > maxRPMGap {
		return learnedState{}, learnedMemoryCandidate{}, false
	}
	similarity := 1 - float64(candidate.rpmGap)/float64(maxRPMGap)
	if similarity <= 0 {
		return learnedState{}, learnedMemoryCandidate{}, false
	}
	migratedOffset := int(math.Round(candidate.state.offset*similarity/float64(max(1, c.config.Step)))) * c.config.Step
	if migratedOffset == 0 {
		return learnedState{}, learnedMemoryCandidate{}, false
	}
	migratedConfidence := clampFloat(0.35+(candidate.state.confidence-0.35)*0.35*similarity, 0.35, 0.45)
	state := learnedState{
		offset:     float64(migratedOffset),
		confidence: migratedConfidence,
		successes:  0,
		failures:   0,
		hasSeed:    false,
	}
	return state, candidate, true
}

func (c *Controller) handleVerifying(zoneIdx int, fanCurve []types.FanCurvePoint, currentTemp, trend int, now time.Time, minRPM, maxRPM int) {
	zone := &c.zones[zoneIdx]
	point := &fanCurve[zoneIdx]
	if currentTemp >= c.config.HighTempBoostThreshold {
		c.enterHighTempProtectionState(zone, currentTemp)
		return
	}

	if currentTemp > zone.verifyTempHigh {
		zone.verifyTempHigh = currentTemp
	}
	if currentTemp < zone.verifyTempLow {
		zone.verifyTempLow = currentTemp
	}

	breakThreshold := c.config.StableDelta * 2
	if currentTemp >= c.config.CriticalTemp {
		breakThreshold = 1
	}

	if trend >= breakThreshold {
		c.noteLearnFailure(point.Temperature, zone, "verify_interrupted_by_heat")
		zone.verifying = false
		zone.stableCount = 0
		oldOffset := point.Offset
		step := c.config.Step
		c.adjustZoneOffset(point, step, minRPM, maxRPM)
		zone.lastAdjustAt = now
		if c.logger != nil {
			c.logger.Debug("风扇偏移验证被升温打断",
				"zone_temp", point.Temperature,
				"trend", trend,
				"old_offset", oldOffset,
				"new_offset", point.Offset,
				"reason", "verify_interrupted_by_heat")
		}
		return
	}
	if zone.verifyTempHigh-zone.verifyTempLow > c.config.VerifyMaxDelta {
		c.noteLearnFailure(point.Temperature, zone, "verify_temp_window_exceeded")
		zone.verifying = false
		zone.stableCount = 0
		return
	}
	verifyScore := c.calculateVerifyScore(point, currentTemp, trend)
	zone.verifyScoreSum += verifyScore
	zone.verifySamples++

	if now.Sub(zone.verifyStartAt) < c.config.VerifyDuration {
		return
	}
	avgScore := 0.0
	if zone.verifySamples > 0 {
		avgScore = zone.verifyScoreSum / float64(zone.verifySamples)
	}
	if avgScore > c.getVerifyScoreThreshold(currentTemp) {
		c.noteLearnFailure(point.Temperature, zone, "verify_score_too_high")
		zone.verifying = false
		zone.stableCount = 0
		if !zone.probeUp {
			point.Offset = zone.probeOffset
		}
		if c.logger != nil {
			c.logger.Debug("风扇偏移验证评分过高",
				"zone_temp", point.Temperature,
				"avg_score", avgScore,
				"offset", point.Offset,
				"reason", "verify_score_too_high")
		}
		return
	}

	zone.verifying = false
	zone.converged = true
	zone.convergeTmp = currentTemp
	zone.stableCount = 0
	zone.driftCount = 0
	c.noteLearnSuccess(point.Temperature, zone, point.Offset)
	if c.logger != nil {
		c.logger.Debug("风扇偏移完成收敛",
			"zone_temp", point.Temperature,
			"offset", point.Offset,
			"has_seed", zone.learnSuccesses > 0,
			"curve_signature", c.lastCurveSignature,
			"verify_samples", zone.verifySamples)
	}
}

func (c *Controller) startVerifying(zone *zoneState, currentTemp int, now time.Time) {
	zone.verifying = true
	zone.verifyStartAt = now
	zone.verifyTempHigh = currentTemp
	zone.verifyTempLow = currentTemp
	zone.verifyScoreSum = 0
	zone.verifySamples = 0
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
				c.logger.Debug("风扇偏移因温度漂移退出收敛",
					"zone_temp", point.Temperature,
					"temp_delta", delta,
					"upward_threshold", upwardThreshold,
					"downward_threshold", downwardThreshold,
					"required_count", requiredCount)
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
		c.logger.Debug("风扇偏移检测到温度瞬变",
			"prev_temp", prevTemp,
			"current_temp", currentTemp,
			"affected_zone_start", lo,
			"affected_zone_end", hi)
	}
	c.tempRing[0] = tempSample{temp: prevTemp}
	c.ringHead = 1 % c.windowSize
	c.ringCount = 1
}

func (c *Controller) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resetRuntimeStateLocked()
}

func (c *Controller) ResetOffsets(fanCurve []types.FanCurvePoint) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resetRuntimeStateLocked()
	for i := range fanCurve {
		fanCurve[i].Offset = 0
	}
}

// ResetForCurve 在曲线基线切换后重置运行状态，并失效旧基线的长期记忆。
func (c *Controller) ResetForCurve(fanCurve []types.FanCurvePoint, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resetForCurveLocked(fanCurve, reason)
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

func (c *Controller) updateEMA(currentTemp int) {
	if !c.emaInitialized {
		c.emaTemp = float64(currentTemp)
		c.emaSlowTemp = float64(currentTemp)
		c.emaTrend = 0
		c.emaInitialized = true
		return
	}
	c.emaTemp = c.config.TempEMAAlpha*float64(currentTemp) + (1-c.config.TempEMAAlpha)*c.emaTemp
	c.emaSlowTemp = c.config.TempEMASlowAlpha*float64(currentTemp) + (1-c.config.TempEMASlowAlpha)*c.emaSlowTemp
	trendSignal := c.emaTemp - c.emaSlowTemp
	c.emaTrend = c.config.TrendEMAAlpha*trendSignal + (1-c.config.TrendEMAAlpha)*c.emaTrend
}

func (c *Controller) getControlTrend() int {
	if !c.emaInitialized {
		return c.calculateTrend()
	}
	return int(math.Round(c.emaTrend))
}

func (c *Controller) processPendingSpike(currentTemp int, fanCurve []types.FanCurvePoint) bool {
	if !c.pendingSpike {
		return false
	}
	baseTemp := c.pendingSpikeBase
	pendingDelta := c.pendingSpikeDelta
	c.pendingSpikeBase = 0
	c.pendingSpikeDelta = 0
	c.pendingSpike = false
	confirmedDelta := currentTemp - baseTemp
	confirmed := sameSign(confirmedDelta, pendingDelta) && iabs(confirmedDelta) >= c.config.SpikeThreshold
	if !confirmed {
		if c.logger != nil {
			c.logger.Debug("风扇偏移忽略候选瞬变", "state", "discarded", "base_temp", baseTemp, "current_temp", currentTemp, "delta", confirmedDelta)
		}
		return true
	}
	c.handleSpike(currentTemp, confirmedDelta, fanCurve)
	return true
}

func (c *Controller) calculateProbeDrop(point *types.FanCurvePoint, currentTemp, stabilityCount, minSafeOffset int) int {
	probeDrop := c.config.Step
	if point.Offset > 0 {
		probeDrop = c.config.Step * 3
	} else if point.Offset > -c.config.Step*5 {
		probeDrop = c.config.Step * 2
	}
	confidence := 0
	if stabilityCount >= c.config.ConvergeCount*2 {
		confidence++
	}
	if currentTemp < c.config.HighTempThreshold-c.config.PreheatBand {
		confidence++
	}
	switch {
	case confidence >= 3:
		probeDrop += c.config.Step * 2
	case confidence >= 2:
		probeDrop += c.config.Step
	case confidence < 0:
		probeDrop = c.config.Step
	}
	maxDrop := point.Offset - minSafeOffset
	if maxDrop < probeDrop {
		probeDrop = maxDrop
	}
	if probeDrop < 0 {
		return 0
	}
	return probeDrop
}

func (c *Controller) describeUpReason(currentTemp, trend, requiredConfirm int) string {
	reason := "ema_trend_rise"
	switch {
	case currentTemp >= c.config.CriticalTemp:
		reason = "critical_temp_force"
	case currentTemp >= c.config.HighTempBoostThreshold:
		reason = "high_temp_boost"
	case currentTemp >= c.config.HighTempThreshold:
		reason = "high_temp_rise"
	}
	if trend >= c.config.StableDelta*3 {
		reason += "+strong_trend"
	}
	if requiredConfirm == 1 {
		reason += "+single_confirm"
	}
	return reason
}

func (c *Controller) applyLearnedOffsetMemory(zone *zoneState, point *types.FanCurvePoint, currentTemp, trend, minSafeOffset int, now time.Time, minRPM, maxRPM int, entrySeed bool) bool {
	if currentTemp >= c.config.HighTempBoostThreshold || point.Temperature >= c.config.HighTempBoostThreshold {
		return false
	}
	if zone.learnedConfidence < 0.35 {
		return false
	}
	if zone.memoryFailStreak >= 2 {
		return false
	}
	if !zone.memoryCooldownUntil.IsZero() && now.Before(zone.memoryCooldownUntil) {
		return false
	}
	targetOffset := int(math.Round(zone.learnedOffset/float64(max(1, c.config.Step)))) * c.config.Step
	if targetOffset < minSafeOffset {
		targetOffset = minSafeOffset
	}
	if currentTemp >= c.config.CriticalTemp && targetOffset < 0 {
		targetOffset = 0
	}
	delta := targetOffset - point.Offset
	if iabs(delta) < c.config.Step {
		return false
	}
	if trend >= c.config.StableDelta*2 && delta < 0 {
		return false
	}
	maxNudge := c.config.Step
	if entrySeed {
		maxNudge = c.learnedMemoryEntryNudge(zone, point, minSafeOffset)
	} else if zone.learnedConfidence >= 0.75 {
		maxNudge = c.config.Step * 2
	}
	delta = clampInt(delta, -maxNudge, maxNudge)
	oldOffset := point.Offset
	c.adjustZoneOffset(point, delta, minRPM, maxRPM)
	if point.Offset == oldOffset {
		return false
	}
	if entrySeed && iabs(targetOffset-point.Offset) < c.config.Step {
		zone.memorySeedPending = false
	} else if !entrySeed {
		zone.memorySeedPending = false
	}
	zone.lastAdjustAt = now
	zone.stableCount = 0
	if c.logger != nil {
		reason := "learned_memory_seed"
		if entrySeed {
			reason += "+entry"
		} else {
			reason += "+resume"
		}
		if delta > 0 {
			reason += "+raise"
		} else {
			reason += "+lower"
		}
		c.logger.Debug("风扇偏移应用长期记忆",
			"zone_temp", point.Temperature,
			"has_seed", zone.learnSuccesses > 0,
			"curve_signature", c.lastCurveSignature,
			"learned_offset", zone.learnedOffset,
			"confidence", zone.learnedConfidence,
			"memory_fail_streak", zone.memoryFailStreak,
			"old_offset", oldOffset,
			"new_offset", point.Offset,
			"reason", reason,
			"trend", trend,
			"raw_temp", currentTemp)
	}
	return true
}

func (c *Controller) learnedMemoryEntryNudge(zone *zoneState, point *types.FanCurvePoint, minSafeOffset int) int {
	maxNudge := c.config.Step * 3
	if zone.learnedConfidence >= 0.55 {
		maxNudge = c.config.Step * 5
	}
	if zone.learnedConfidence >= 0.75 {
		maxNudge = c.config.Step * 6
	}
	if zone.learnedConfidence >= 0.85 {
		maxNudge += c.config.Step * 2
	}
	targetOffset := int(math.Round(zone.learnedOffset/float64(max(1, c.config.Step)))) * c.config.Step
	if targetOffset < minSafeOffset {
		targetOffset = minSafeOffset
	}
	availableDrop := point.Offset - targetOffset
	if availableDrop > 0 && availableDrop < maxNudge {
		maxNudge = availableDrop
	}
	if maxNudge < c.config.Step {
		return c.config.Step
	}
	return maxNudge
}

func (c *Controller) hasHighTempStagnationPressure(zone *zoneState, currentTemp int) bool {
	if currentTemp >= c.config.HighTempBoostThreshold {
		return true
	}
	if zone.memorySeedPending {
		return false
	}
	if zone.learnedConfidence >= 0.35 && zone.learnedOffset < 0 {
		return false
	}
	if c.emaInitialized && c.emaTrend >= float64(max(1, c.config.StableDelta)) {
		return true
	}
	return false
}

func (c *Controller) shouldProbeUp(point *types.FanCurvePoint, currentTemp int) bool {
	if currentTemp >= c.config.HighTempBoostThreshold || point.Temperature >= c.config.HighTempBoostThreshold {
		return false
	}
	if currentTemp >= c.config.CriticalTemp {
		return false
	}
	if currentTemp < c.config.HighTempThreshold-c.config.PreheatBand {
		return false
	}
	if point.Offset >= c.config.Step*6 {
		return false
	}
	pressure := 0
	if c.emaTrend >= float64(max(1, c.config.StableDelta)) {
		pressure++
	}
	if currentTemp >= c.config.HighTempThreshold {
		pressure++
	}
	return pressure >= 2
}

func (c *Controller) noteLearnSuccess(temp int, zone *zoneState, offset int) {
	if temp >= c.config.HighTempBoostThreshold {
		c.clearLearnState(zone, temp)
		return
	}
	if c.learnedMemory == nil {
		c.learnedMemory = newLearnedMemoryStore()
	}
	key := learnedMemoryKey{curveSignature: c.lastCurveSignature, zoneTemp: temp}
	state, _ := c.learnedMemory.Get(key.curveSignature, key.zoneTemp)
	hadSeed := state.hasSeed
	if !hadSeed {
		state.offset = float64(offset)
	} else {
		rate := clampFloat(c.config.LearnRate*(0.6+0.4*state.confidence), 0.05, 0.5)
		state.offset = (1-rate)*state.offset + rate*float64(offset)
	}
	state.hasSeed = true
	state.confidence = clampFloat(state.confidence+c.config.LearnConfidenceGain, 0, 1)
	state.successes++
	c.learnedMemory.Set(key.curveSignature, key.zoneTemp, state)
	c.persistLearnedStateLocked(key.curveSignature, key.zoneTemp, state)
	c.applyLearnedStateToZone(zone, state)
	c.clearLearnedMemorySuppression(zone)
	if c.logger != nil {
		c.logger.Debug("风扇偏移长期记忆学习成功",
			"zone_temp", temp,
			"has_seed", hadSeed,
			"curve_signature", key.curveSignature,
			"offset", offset,
			"learned_offset", state.offset,
			"confidence", state.confidence,
			"successes", state.successes,
			"failures", state.failures)
	}
}

func (c *Controller) noteLearnFailure(temp int, zone *zoneState, reason string) {
	if temp >= c.config.HighTempBoostThreshold {
		c.clearLearnState(zone, temp)
		return
	}
	now := time.Now()
	cooldown := c.suppressLearnedMemory(zone, now)
	if c.learnedMemory == nil {
		c.learnedMemory = newLearnedMemoryStore()
	}
	key := learnedMemoryKey{curveSignature: c.lastCurveSignature, zoneTemp: temp}
	state, _ := c.learnedMemory.Get(key.curveSignature, key.zoneTemp)
	state.confidence = clampFloat(state.confidence-c.config.LearnConfidenceDecay, 0, 1)
	state.failures++
	c.learnedMemory.Set(key.curveSignature, key.zoneTemp, state)
	c.persistLearnedStateLocked(key.curveSignature, key.zoneTemp, state)
	c.applyLearnedStateToZone(zone, state)
	if c.logger != nil {
		c.logger.Debug("风扇偏移长期记忆学习失败",
			"zone_temp", temp,
			"reason", reason,
			"has_seed", state.hasSeed,
			"curve_signature", key.curveSignature,
			"learned_offset", state.offset,
			"confidence", state.confidence,
			"memory_fail_streak", zone.memoryFailStreak,
			"memory_cooldown", cooldown,
			"successes", state.successes,
			"failures", state.failures)
	}
}

func (c *Controller) suppressLearnedMemory(zone *zoneState, now time.Time) time.Duration {
	if now.IsZero() {
		now = time.Now()
	}
	zone.memoryFailStreak++
	zone.memorySeedPending = false
	cooldown := c.learnedMemoryCooldown(zone.memoryFailStreak)
	until := now.Add(cooldown)
	if until.After(zone.memoryCooldownUntil) {
		zone.memoryCooldownUntil = until
	}
	return cooldown
}

func (c *Controller) clearLearnedMemorySuppression(zone *zoneState) {
	zone.memoryCooldownUntil = time.Time{}
	zone.memoryFailStreak = 0
}

func (c *Controller) learnedMemoryCooldown(failStreak int) time.Duration {
	cooldown := c.config.VerifyDuration / 2
	if cooldown < c.config.AdjustCooldown*2 {
		cooldown = c.config.AdjustCooldown * 2
	}
	if cooldown < 15*time.Second {
		cooldown = 15 * time.Second
	}
	if failStreak > 1 {
		cooldown += time.Duration(failStreak-1) * c.config.AdjustCooldown
	}
	return cooldown
}

func (c *Controller) calculateProbeUpStep(currentTemp int) int {
	step := c.config.Step
	if currentTemp >= c.config.HighTempBoostThreshold {
		return step * 2
	}
	if currentTemp >= c.config.HighTempThreshold {
		return int(math.Round(float64(step) * 1.5))
	}
	return step
}

func (c *Controller) getRequiredTrendConfirm(currentTemp, trend int) int {
	if currentTemp >= c.config.CriticalTemp {
		return 1
	}
	confirm := float64(max(1, c.config.HighTempTrendConfirm))
	trendStrength := math.Abs(float64(trend)) / float64(max(1, c.config.StableDelta))
	var tempFactor float64
	switch {
	case currentTemp >= c.config.HighTempBoostThreshold:
		rangeSpan := float64(max(1, c.config.CriticalTemp-c.config.HighTempBoostThreshold))
		progress := clampFloat(float64(currentTemp-c.config.HighTempBoostThreshold)/rangeSpan, 0, 1)
		tempFactor = 2.2 - 1.2*progress
	case currentTemp >= c.config.HighTempThreshold:
		rangeSpan := float64(max(1, c.config.HighTempBoostThreshold-c.config.HighTempThreshold))
		progress := clampFloat(float64(currentTemp-c.config.HighTempThreshold)/rangeSpan, 0, 1)
		tempFactor = 4.0 - 1.8*progress
	default:
		rangeSpan := float64(max(1, c.config.HighTempThreshold-c.config.SafeTemp))
		progress := clampFloat(float64(currentTemp-c.config.SafeTemp)/rangeSpan, 0, 1)
		tempFactor = 4.8 - 2.4*progress
	}
	var trendFactor float64
	switch {
	case trendStrength >= 3:
		trendFactor = 0.75
	case trendStrength >= 2:
		trendFactor = 1.0
	case trendStrength >= 1.5:
		trendFactor = 1.25
	default:
		trendFactor = 1.6
	}
	return max(1, int(math.Round(confirm*tempFactor*trendFactor)))
}

func (c *Controller) calculateVerifyScore(point *types.FanCurvePoint, currentTemp, trend int) float64 {
	score := 0.0
	if currentTemp > point.Temperature {
		score += float64(currentTemp-point.Temperature) * 0.45
	}
	if trend > 0 {
		score += float64(trend) * 0.9
	}
	safetyMargin := c.config.CriticalTemp - currentTemp
	if safetyMargin < 10 {
		score += float64(10-safetyMargin) * 0.5
	}
	return score
}

func (c *Controller) getVerifyScoreThreshold(currentTemp int) float64 {
	switch {
	case currentTemp >= c.config.HighTempBoostThreshold:
		return 3.0
	case currentTemp >= c.config.HighTempThreshold:
		return 3.8
	default:
		return 4.6
	}
}

func (c *Controller) applyPreheatClamp(minSafeOffset, currentTemp int) int {
	if !c.emaInitialized || currentTemp >= c.config.HighTempThreshold {
		return minSafeOffset
	}
	preheatStart := c.config.HighTempThreshold - c.config.PreheatBand
	if preheatStart >= c.config.HighTempThreshold {
		return minSafeOffset
	}
	controlTemp := math.Max(c.emaTemp, float64(currentTemp))
	if controlTemp < float64(preheatStart) || c.emaTrend <= 0 {
		return minSafeOffset
	}
	tempSpan := float64(max(1, c.config.HighTempThreshold-preheatStart))
	tempProgress := clampFloat((controlTemp-float64(preheatStart))/tempSpan, 0, 1)
	trendProgress := clampFloat(c.emaTrend/float64(max(1, c.config.StableDelta*2)), 0, 1)
	clampStrength := 0.35 + tempProgress*0.9 + trendProgress*0.75
	clamp := int(math.Round(float64(c.config.PreheatClampStep) * clampStrength))
	if clamp <= 0 {
		return minSafeOffset
	}
	if minSafeOffset < 0 {
		minSafeOffset += clamp
		if minSafeOffset > 0 {
			return 0
		}
	}
	return minSafeOffset
}

func (c *Controller) resetRuntimeStateLocked() {
	c.ringHead = 0
	c.ringCount = 0
	c.lastTemp = -1
	c.lastTempChangedAt = time.Now()
	c.emaTemp = 0
	c.emaSlowTemp = 0
	c.emaTrend = 0
	c.emaInitialized = false
	c.pendingSpikeBase = 0
	c.pendingSpikeDelta = 0
	c.pendingSpike = false
	c.activeZoneTemp = 0
	c.activeZoneValid = false
	c.lastCurveTemps = c.lastCurveTemps[:0]
	c.lastCurveSignature = ""
	for i := range c.zones {
		c.zones[i] = zoneState{}
	}
}

func (c *Controller) resetForCurveLocked(fanCurve []types.FanCurvePoint, reason string) {
	curveSignature := c.curveSignature(fanCurve)
	previousSignature := c.storedCurveSignatureLocked()
	hadSeed := c.hasLearnedSeedLocked()
	retained := 0
	available := 0
	if previousSignature != "" && previousSignature != curveSignature && c.learnedMemory != nil {
		retained = c.learnedMemory.CountForCurve(previousSignature)
		available = c.learnedMemory.CountForCurve(curveSignature)
	}
	c.resetRuntimeStateLocked()
	if previousSignature != "" && previousSignature != curveSignature && c.logger != nil {
		c.logger.Info("风扇偏移曲线基线变更，重置自适应状态",
			"reason", reason,
			"has_seed", hadSeed,
			"previous_curve_signature", previousSignature,
			"curve_signature", curveSignature,
			"retained_memory", retained,
			"available_memory", available)
	}
}

func (c *Controller) storedCurveSignatureLocked() string {
	if c.lastCurveSignature != "" {
		return c.lastCurveSignature
	}
	if c.learnedMemory == nil {
		return ""
	}
	return ""
}

func (c *Controller) hasLearnedSeedLocked() bool {
	return c.learnedMemory.HasSeedForCurve(c.storedCurveSignatureLocked())
}

func (c *Controller) curveSignature(fanCurve []types.FanCurvePoint) string {
	if len(fanCurve) == 0 {
		return ""
	}
	var builder strings.Builder
	for i, point := range fanCurve {
		if i > 0 {
			builder.WriteByte('|')
		}
		builder.WriteString(strconv.Itoa(point.Temperature))
		builder.WriteByte(':')
		builder.WriteString(strconv.Itoa(point.RPM))
	}
	return builder.String()
}

// ensureZones 按温度点匹配继承已有zoneState
func (c *Controller) ensureZones(fanCurve []types.FanCurvePoint) {
	n := len(fanCurve)
	curveSignature := c.curveSignature(fanCurve)
	previousZones := len(c.zones)
	previousSignature := c.storedCurveSignatureLocked()
	if previousSignature != "" && previousSignature != curveSignature {
		hadSeed := c.hasLearnedSeedLocked()
		retained := c.learnedMemory.CountForCurve(previousSignature)
		available := c.learnedMemory.CountForCurve(curveSignature)
		c.resetRuntimeStateLocked()
		if c.logger != nil {
			c.logger.Info("风扇偏移检测到曲线基线变化",
				"reason", "curve_signature_changed",
				"has_seed", hadSeed,
				"previous_curve_signature", previousSignature,
				"curve_signature", curveSignature,
				"retained_memory", retained,
				"available_memory", available)
		}
	}
	if len(c.zones) == n {
		changed := len(c.lastCurveTemps) != n || c.lastCurveSignature != curveSignature
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
		newZones[i].learnedOffset = 0
		newZones[i].learnedConfidence = 0
		newZones[i].learnSuccesses = 0
		newZones[i].learnFailures = 0
		if p.Temperature < c.config.HighTempBoostThreshold {
			if learned, ok := c.learnedMemory.Get(curveSignature, p.Temperature); ok {
				c.applyLearnedStateToZone(&newZones[i], learned)
			} else if migrated, source, ok := c.buildMigratedLearnedState(curveSignature, p); ok {
				c.applyLearnedStateToZone(&newZones[i], migrated)
				if c.logger != nil {
					c.logger.Debug("风扇偏移短期迁移命中",
						"zone_temp", p.Temperature,
						"curve_signature", curveSignature,
						"source_curve_signature", source.curveSignature,
						"target_rpm", p.RPM,
						"source_rpm", source.rpm,
						"rpm_gap", source.rpmGap,
						"source_offset", source.state.offset,
						"migrated_offset", migrated.offset,
						"migrated_confidence", migrated.confidence)
				}
			}
		}
	}

	if c.logger != nil && (previousZones != n || reset > 0) {
		c.logger.Debug("风扇偏移曲线节点变更",
			"state", "zone_reset",
			"previous", previousZones,
			"current", n,
			"inherited", inherited,
			"reset", reset,
			"curve_signature", curveSignature)
	}

	c.zones = newZones
	c.lastCurveSignature = curveSignature

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

func sameSign(a, b int) bool {
	return (a > 0 && b > 0) || (a < 0 && b < 0)
}

func clampInt(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func clampFloat(v, low, high float64) float64 {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func (c *Controller) persistLearnedStateLocked(curveSignature string, zoneTemp int, state learnedState) {
	if c.persistence == nil || curveSignature == "" {
		return
	}
	shouldSnapshot, err := c.persistence.RecordSet(curveSignature, zoneTemp, state)
	if err != nil {
		if c.logger != nil {
			c.logger.Error("风扇偏移长期记忆写入 journal 失败", "curve_signature", curveSignature, "zone_temp", zoneTemp, "error", err)
		}
		return
	}
	if shouldSnapshot {
		_ = c.flushPersistenceLocked("journal_threshold")
	}
}

func (c *Controller) persistLearnedDeleteLocked(curveSignature string, zoneTemp int) {
	if c.persistence == nil || curveSignature == "" {
		return
	}
	shouldSnapshot, err := c.persistence.RecordDelete(curveSignature, zoneTemp)
	if err != nil {
		if c.logger != nil {
			c.logger.Error("风扇偏移长期记忆删除 journal 失败", "curve_signature", curveSignature, "zone_temp", zoneTemp, "error", err)
		}
		return
	}
	if shouldSnapshot {
		_ = c.flushPersistenceLocked("journal_threshold")
	}
}

func (c *Controller) flushPersistenceLocked(reason string) error {
	if c.persistence == nil || c.learnedMemory == nil {
		return nil
	}
	if err := c.persistence.Snapshot(c.learnedMemory); err != nil {
		if c.logger != nil {
			c.logger.Error("风扇偏移长期记忆写入 snapshot 失败", "reason", reason, "error", err)
		}
		return err
	}
	if c.logger != nil && reason != "manual" {
		c.logger.Debug("风扇偏移长期记忆完成 snapshot", "reason", reason, "entries", c.learnedMemory.Len())
	}
	return nil
}
