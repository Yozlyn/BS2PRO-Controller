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
	fanCommandResendCooldown  = 1500 * time.Millisecond
	fanCommandModeRecoveryGap = 4 * time.Second
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
	lastCommandTargetRPM int
	lastSentAt           time.Time
	lastShapeReason      string
	lastModeRecoveryAt   time.Time
}

func newFanCommandShaper() *fanCommandShaper {
	return &fanCommandShaper{}
}

func (a *CoreApp) resetFanRuntimeState() {
	if a.fanOffsetCtrl != nil {
		a.fanOffsetCtrl.Reset()
	}
	a.clearRuntimeFanCurve()
	a.resetFanCommandShaper()
}

func (a *CoreApp) resetFanOffsetRuntimeState() {
	if a.fanOffsetCtrl != nil {
		a.fanOffsetCtrl.Reset()
	}
	a.clearRuntimeFanCurve()
}

func (a *CoreApp) resetFanCommandShaper() {
	if a.fanCommandShaper != nil {
		a.fanCommandShaper.Reset()
	}
}

func (a *CoreApp) planFanCommandTarget(now time.Time, thermalTargetRPM int, latestFanData *types.FanData) fanCommandPlan {
	if a.fanCommandShaper == nil {
		commandTargetRPM := normalizeFanCommandRPM(thermalTargetRPM)
		return fanCommandPlan{
			ThermalTargetRPM: commandTargetRPM,
			CommandTargetRPM: commandTargetRPM,
			ShouldSend:       commandTargetRPM > 0,
			ShapeReason:      "target_follow",
		}
	}
	return a.fanCommandShaper.Plan(now, thermalTargetRPM, latestFanData)
}

func (s *fanCommandShaper) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastCommandRPM = 0
	s.lastCommandTargetRPM = 0
	s.lastSentAt = time.Time{}
	s.lastShapeReason = ""
	s.lastModeRecoveryAt = time.Time{}
}

func (s *fanCommandShaper) Plan(now time.Time, thermalTargetRPM int, latestFanData *types.FanData) fanCommandPlan {
	plan := fanCommandPlan{ThermalTargetRPM: normalizeFanCommandRPM(thermalTargetRPM)}
	if plan.ThermalTargetRPM <= 0 {
		return plan
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	plan.CommandTargetRPM = plan.ThermalTargetRPM
	plan.ShapeReason = "target_follow"
	plan.StateChanged = plan.ShapeReason != s.lastShapeReason || plan.CommandTargetRPM != s.lastCommandTargetRPM
	s.lastShapeReason = plan.ShapeReason
	s.lastCommandTargetRPM = plan.CommandTargetRPM
	plan.ShouldSend = s.shouldSendLocked(now, plan.CommandTargetRPM, latestFanData)
	if plan.ShouldSend {
		s.lastCommandRPM = plan.CommandTargetRPM
		s.lastSentAt = now
	}
	return plan
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
