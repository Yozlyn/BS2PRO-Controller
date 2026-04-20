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
	fanCommandDropHotStepRPM  = 100
	fanCommandCoolingHold     = 12 * time.Second
	fanCommandResendCooldown  = 1500 * time.Millisecond
)

type fanCommandPlan struct {
	ThermalTargetRPM int
	CommandTargetRPM int
	ShouldSend       bool
	ShapeReason      string
	StateChanged     bool
}

type fanCommandShaper struct {
	mu sync.Mutex

	lastCommandRPM       int
	lastThermalTargetRPM int
	lastTemp             int
	lastSentAt           time.Time
	lastShapeReason      string

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
	s.lastThermalTargetRPM = 0
	s.lastTemp = -1
	s.lastSentAt = time.Time{}
	s.lastShapeReason = ""
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

	thermalRising := s.lastThermalTargetRPM > 0 && plan.ThermalTargetRPM > s.lastThermalTargetRPM
	tempRising := s.lastTemp >= 0 && currentTemp > s.lastTemp
	if plan.ThermalTargetRPM >= referenceRPM || thermalRising || tempRising {
		s.coolingHoldUntil = time.Time{}
		s.coolingDecayActive = false
	}

	plan.CommandTargetRPM = plan.ThermalTargetRPM
	plan.ShapeReason = "steady"

	switch {
	case plan.ThermalTargetRPM > referenceRPM:
		step := fanCommandRiseStepRPM
		gap := plan.ThermalTargetRPM - referenceRPM
		switch {
		case currentTemp >= 90 || gap >= 1800:
			step = fanCommandCriticalStepRPM
		case currentTemp >= 85 || gap >= 1200:
			step = fanCommandBoostStepRPM
		}
		plan.CommandTargetRPM = min(plan.ThermalTargetRPM, referenceRPM+step)
		if plan.CommandTargetRPM < plan.ThermalTargetRPM {
			plan.ShapeReason = "ramp_up"
		} else {
			plan.ShapeReason = "rise_follow"
		}
	case plan.ThermalTargetRPM < referenceRPM:
		if !s.coolingDecayActive {
			if s.coolingHoldUntil.IsZero() {
				s.coolingHoldUntil = now.Add(fanCommandCoolingHold)
			}
			if now.Before(s.coolingHoldUntil) {
				plan.CommandTargetRPM = referenceRPM
				plan.ShapeReason = "cooling_hold"
				break
			}
			s.coolingHoldUntil = time.Time{}
			s.coolingDecayActive = true
		}

		dropStep := fanCommandDropStepRPM
		if currentTemp >= 80 {
			dropStep = fanCommandDropHotStepRPM
		}
		plan.CommandTargetRPM = max(plan.ThermalTargetRPM, referenceRPM-dropStep)
		if plan.CommandTargetRPM > plan.ThermalTargetRPM {
			plan.ShapeReason = "ramp_down"
		} else {
			plan.ShapeReason = "cool_follow"
			s.coolingDecayActive = false
		}
	default:
		plan.CommandTargetRPM = referenceRPM
		plan.ShapeReason = "steady"
		s.coolingHoldUntil = time.Time{}
		s.coolingDecayActive = false
	}

	plan.CommandTargetRPM = normalizeFanCommandRPM(plan.CommandTargetRPM)
	plan.StateChanged = plan.ShapeReason != s.lastShapeReason
	s.lastShapeReason = plan.ShapeReason
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
