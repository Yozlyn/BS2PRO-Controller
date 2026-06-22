package main

import (
	"github.com/TIANLI0/BS2PRO-Controller/internal/ipc"
	"github.com/TIANLI0/BS2PRO-Controller/internal/types"
)

func (a *CoreApp) setRuntimeFanCurve(curve []types.FanCurvePoint) bool {
	clone := cloneFanCurvePoints(curve)
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if fanCurvesEqual(a.runtimeFanCurve, clone) {
		return false
	}
	a.runtimeFanCurve = clone
	return true
}

func (a *CoreApp) clearRuntimeFanCurve() {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.clearRuntimeFanCurveLocked()
}

func (a *CoreApp) clearRuntimeFanCurveLocked() {
	if len(a.runtimeFanCurve) == 0 {
		return
	}
	a.runtimeFanCurve = nil
}

func (a *CoreApp) visibleFanCurve(base []types.FanCurvePoint) []types.FanCurvePoint {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	if len(a.runtimeFanCurve) > 0 {
		return cloneFanCurvePoints(a.runtimeFanCurve)
	}
	return cloneFanCurvePoints(base)
}

func (a *CoreApp) broadcastConfigUpdate(cfg types.AppConfig) {
	if a.ipcServer == nil {
		return
	}
	a.ipcServer.BroadcastEvent(ipc.EventConfigUpdate, cfg)
}

func (a *CoreApp) broadcastVisibleFanCurve(base []types.FanCurvePoint) {
	if a.ipcServer == nil {
		return
	}
	a.ipcServer.BroadcastEvent(ipc.EventFanCurveUpdate, a.visibleFanCurve(base))
}

func fanCurvesEqual(a, b []types.FanCurvePoint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Temperature != b[i].Temperature || a[i].RPM != b[i].RPM || a[i].Offset != b[i].Offset {
			return false
		}
	}
	return true
}
