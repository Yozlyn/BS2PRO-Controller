package main

import (
	"sync"
	"time"

	"github.com/TIANLI0/BS2PRO-Controller/internal/types"
)

const (
	fanCommandStepRPM         = 100
	fanCommandMinRPM          = 500
	fanCommandMaxRPM          = 4000
	fanCommandRiseStepRPM     = 400
	fanCommandBoostStepRPM    = 700
	fanCommandCriticalStepRPM = 900
	fanCommandDropStepRPM     = 200
	fanCommandDropBoostRPM    = 400
	fanCommandDropFastRPM     = 700
	fanCommandDropHotStepRPM  = 100
	fanCommandCoolingHold     = 12 * time.Second
	fanCommandCoolingSoftHold = 6 * time.Second
	fanCommandCoolingFastHold = 3 * time.Second
	fanCommandResendCooldown  = 1500 * time.Millisecond
	fanCommandModeRecoveryGap = 4 * time.Second
	fanCommandHotHoldTemp     = 80
	fanCommandWarmHoldTemp    = 76
	fanCommandFastCoolTemp    = 70
	fanCommandSoftCoolGapRPM  = 900
	fanCommandFastCoolGapRPM  = 1600
)

type fanCommandDirection string

const (
	fanCommandDirectionSteady  fanCommandDirection = "steady"
	fanCommandDirectionHeating fanCommandDirection = "heating"
	fanCommandDirectionCooling fanCommandDirection = "cooling"
)

type fanCommandPlan struct {
	ThermalTargetRPM int
	CommandTargetRPM int
	ShouldSend       bool
	ShapeReason      string
	StateChanged     bool
}

type fanCommandShapeState struct {
	coolingHoldUntil   time.Time
	coolingDecayActive bool
	lastAboveThreshold time.Time
}

type fanCommandShaper struct {
	mu sync.Mutex

	lastCommandRPM       int
	lastCommandTargetRPM int
	lastThermalTargetRPM int
	lastTemp             int
	lastSentAt           time.Time
	lastShapeReason      string
	lastModeRecoveryAt   time.Time
	lastThermalDirection fanCommandDirection
	lastAboveThresholdAt time.Time

	coolingHoldUntil   time.Time
	coolingDecayActive bool
}

func newFanCommandShaper() *fanCommandShaper {
	return &fanCommandShaper{lastTemp: -1}
}

func (a *CoreApp) resetFanRuntimeState() {
	if a.fanOffsetCtrl != nil {
		a.fanOffsetCtrl.Reset()
	}
	a.clearRuntimeFanCurve()
	a.resetFanCommandShaper()
}

func (a *CoreApp) resetFanCommandShaper() {
	if a.fanCommandShaper != nil {
		a.fanCommandShaper.Reset()
	}
}

func (a *CoreApp) planFanCommandTarget(now time.Time, thermalTargetRPM, currentTemp int, latestFanData *types.FanData) fanCommandPlan {
	if a.fanCommandShaper == nil {
		commandTargetRPM := normalizeFanCommandRPM(thermalTargetRPM)
		return fanCommandPlan{
			ThermalTargetRPM: commandTargetRPM,
			CommandTargetRPM: commandTargetRPM,
			ShouldSend:       commandTargetRPM > 0,
			ShapeReason:      "direct",
		}
	}
	return a.fanCommandShaper.Plan(now, thermalTargetRPM, currentTemp, latestFanData)
}

func (s *fanCommandShaper) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastCommandRPM = 0
	s.lastCommandTargetRPM = 0
	s.lastThermalTargetRPM = 0
	s.lastTemp = -1
	s.lastSentAt = time.Time{}
	s.lastShapeReason = ""
	s.lastModeRecoveryAt = time.Time{}
	s.lastThermalDirection = fanCommandDirectionSteady
	s.lastAboveThresholdAt = time.Time{}
	s.coolingHoldUntil = time.Time{}
	s.coolingDecayActive = false
}

func (s *fanCommandShaper) Plan(now time.Time, thermalTargetRPM, currentTemp int, latestFanData *types.FanData) fanCommandPlan {
	plan := fanCommandPlan{ThermalTargetRPM: normalizeFanCommandRPM(thermalTargetRPM)}
	if plan.ThermalTargetRPM <= 0 {
		return plan
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	referenceRPM := s.referenceRPMLocked(latestFanData)
	if referenceRPM <= 0 {
		referenceRPM = plan.ThermalTargetRPM
	}

	direction := s.thermalDirectionLocked(plan.ThermalTargetRPM, referenceRPM)
	shapeState := fanCommandShapeState{
		coolingHoldUntil:   s.coolingHoldUntil,
		coolingDecayActive: s.coolingDecayActive,
		lastAboveThreshold: s.lastAboveThresholdAt,
	}
	plan.CommandTargetRPM, plan.ShapeReason, shapeState = shapeCommandRPM(now, referenceRPM, plan.ThermalTargetRPM, currentTemp, direction, shapeState)
	s.coolingHoldUntil = shapeState.coolingHoldUntil
	s.coolingDecayActive = shapeState.coolingDecayActive
	s.lastAboveThresholdAt = shapeState.lastAboveThreshold

	plan.CommandTargetRPM = normalizeFanCommandRPM(plan.CommandTargetRPM)
	plan.StateChanged = plan.ShapeReason != s.lastShapeReason || direction != s.lastThermalDirection || plan.CommandTargetRPM != s.lastCommandTargetRPM
	s.lastShapeReason = plan.ShapeReason
	s.lastThermalDirection = direction
	s.lastCommandTargetRPM = plan.CommandTargetRPM
	plan.ShouldSend = s.shouldSendLocked(now, plan.CommandTargetRPM, latestFanData)
	if plan.ShouldSend {
		s.lastCommandRPM = plan.CommandTargetRPM
		s.lastSentAt = now
	}
	s.lastThermalTargetRPM = plan.ThermalTargetRPM
	s.lastTemp = currentTemp
	return plan
}

func (s *fanCommandShaper) referenceRPMLocked(latestFanData *types.FanData) int {
	referenceRPM := normalizeFanCommandRPM(s.lastCommandRPM)
	if latestFanData == nil {
		return referenceRPM
	}
	if deviceTargetRPM := normalizeFanCommandRPM(int(latestFanData.TargetRPM)); deviceTargetRPM > referenceRPM {
		referenceRPM = deviceTargetRPM
	}
	if referenceRPM <= 0 {
		referenceRPM = normalizeFanCommandRPM(int(latestFanData.CurrentRPM))
	}
	return referenceRPM
}

func (s *fanCommandShaper) shouldSendLocked(now time.Time, commandTargetRPM int, latestFanData *types.FanData) bool {
	if commandTargetRPM <= 0 {
		return false
	}
	if latestFanData != nil {
		if latestFanData.WorkMode == "挡位工作模式" {
			return true
		}
		if deviceTargetRPM := normalizeFanCommandRPM(int(latestFanData.TargetRPM)); deviceTargetRPM > 0 && absInt(commandTargetRPM-deviceTargetRPM) < fanCommandStepRPM {
			return false
		}
	}
	if s.lastCommandRPM > 0 && absInt(commandTargetRPM-s.lastCommandRPM) < fanCommandStepRPM && !s.lastSentAt.IsZero() && now.Sub(s.lastSentAt) < fanCommandResendCooldown {
		return false
	}
	return true
}

func (s *fanCommandShaper) AllowModeRecovery(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.lastModeRecoveryAt.IsZero() && now.Sub(s.lastModeRecoveryAt) < fanCommandModeRecoveryGap {
		return false
	}
	s.lastModeRecoveryAt = now
	return true
}

func (s *fanCommandShaper) thermalDirectionLocked(thermalTargetRPM, referenceRPM int) fanCommandDirection {
	switch {
	case thermalTargetRPM > referenceRPM:
		return fanCommandDirectionHeating
	case thermalTargetRPM < referenceRPM:
		return fanCommandDirectionCooling
	case s.lastThermalTargetRPM > 0 && thermalTargetRPM > s.lastThermalTargetRPM:
		return fanCommandDirectionHeating
	case s.lastThermalTargetRPM > 0 && thermalTargetRPM < s.lastThermalTargetRPM:
		return fanCommandDirectionCooling
	default:
		return fanCommandDirectionSteady
	}
}

func shapeCommandRPM(now time.Time, referenceRPM, thermalTargetRPM, currentTemp int, direction fanCommandDirection, state fanCommandShapeState) (int, string, fanCommandShapeState) {
	if currentTemp >= fanCommandHotHoldTemp {
		state.lastAboveThreshold = now
	}

	switch direction {
	case fanCommandDirectionHeating:
		state.coolingHoldUntil = time.Time{}
		state.coolingDecayActive = false
		step := fanCommandRiseStepRPM
		gap := thermalTargetRPM - referenceRPM
		switch {
		case currentTemp >= 90 || gap >= 1800:
			step = fanCommandCriticalStepRPM
		case currentTemp >= 85 || gap >= 1200:
			step = fanCommandBoostStepRPM
		}
		commandRPM := min(thermalTargetRPM, referenceRPM+step)
		if commandRPM < thermalTargetRPM {
			return commandRPM, "ramp_up", state
		}
		return commandRPM, "rise_follow", state
	case fanCommandDirectionCooling:
		if !state.coolingDecayActive {
			holdDuration := fanCommandCoolingDuration(referenceRPM, thermalTargetRPM, currentTemp)
			holdUntil := state.coolingHoldUntil
			if holdUntil.IsZero() {
				holdUntil = now.Add(holdDuration)
			}
			if !state.lastAboveThreshold.IsZero() && shouldExtendFanCommandThresholdHold(referenceRPM, thermalTargetRPM, currentTemp) {
				thresholdHoldUntil := state.lastAboveThreshold.Add(fanCommandCoolingHold)
				if thresholdHoldUntil.After(holdUntil) {
					holdUntil = thresholdHoldUntil
				}
			}
			state.coolingHoldUntil = holdUntil
			if now.Before(state.coolingHoldUntil) {
				return referenceRPM, "cooling_hold", state
			}
			state.coolingHoldUntil = time.Time{}
			state.coolingDecayActive = true
		}

		dropStep := fanCommandCoolingDropStep(referenceRPM, thermalTargetRPM, currentTemp)
		commandRPM := max(thermalTargetRPM, referenceRPM-dropStep)
		if commandRPM > thermalTargetRPM {
			return commandRPM, "ramp_down", state
		}
		state.coolingDecayActive = false
		return commandRPM, "cool_follow", state
	default:
		state.coolingHoldUntil = time.Time{}
		state.coolingDecayActive = false
		return referenceRPM, "steady", state
	}
}

func fanCommandCoolingDuration(referenceRPM, thermalTargetRPM, currentTemp int) time.Duration {
	gap := max(0, referenceRPM-thermalTargetRPM)
	switch {
	case currentTemp >= fanCommandHotHoldTemp:
		return fanCommandCoolingHold
	case currentTemp <= fanCommandFastCoolTemp && gap >= fanCommandFastCoolGapRPM:
		return fanCommandCoolingFastHold
	case currentTemp <= fanCommandWarmHoldTemp || gap >= fanCommandSoftCoolGapRPM:
		return fanCommandCoolingSoftHold
	default:
		return fanCommandCoolingHold
	}
}

func shouldExtendFanCommandThresholdHold(referenceRPM, thermalTargetRPM, currentTemp int) bool {
	gap := max(0, referenceRPM-thermalTargetRPM)
	return currentTemp >= fanCommandWarmHoldTemp || gap < fanCommandSoftCoolGapRPM
}

func fanCommandCoolingDropStep(referenceRPM, thermalTargetRPM, currentTemp int) int {
	gap := max(0, referenceRPM-thermalTargetRPM)
	switch {
	case currentTemp >= fanCommandHotHoldTemp:
		return fanCommandDropHotStepRPM
	case gap >= fanCommandFastCoolGapRPM:
		return fanCommandDropFastRPM
	case gap >= fanCommandSoftCoolGapRPM:
		return fanCommandDropBoostRPM
	default:
		return fanCommandDropStepRPM
	}
}

func normalizeFanCommandRPM(rpm int) int {
	if rpm <= 0 {
		return 0
	}
	rpm = ((rpm + fanCommandStepRPM/2) / fanCommandStepRPM) * fanCommandStepRPM
	if rpm < fanCommandMinRPM {
		return fanCommandMinRPM
	}
	if rpm > fanCommandMaxRPM {
		return fanCommandMaxRPM
	}
	return rpm
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
