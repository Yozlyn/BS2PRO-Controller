//go:build windows

package main

import (
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"github.com/TIANLI0/BS2PRO-Controller/internal/types"
	"golang.org/x/sys/windows"
)

const errorHotkeyAlreadyRegistered syscall.Errno = 1409

const (
	wmHotkey    = 0x0312
	modAlt      = 0x0001
	modCtrl     = 0x0002
	modShift    = 0x0004
	modWin      = 0x0008
	modNoRepeat = 0x4000
)

var (
	hotkeyUser32       = windows.NewLazySystemDLL("user32.dll")
	procRegisterHotKey = hotkeyUser32.NewProc("RegisterHotKey")
	procUnregister     = hotkeyUser32.NewProc("UnregisterHotKey")
	procGetMessage     = hotkeyUser32.NewProc("GetMessageW")
	procPostThreadMsg  = hotkeyUser32.NewProc("PostThreadMessageW")
)

const wmQuit = 0x0012

type point struct{ x, y int32 }

type msg struct {
	hwnd     uintptr
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	pt       point
	lPrivate uint32
}

func registerGlobalHotkeys(bindings []types.HotkeyBinding, onAction func(string)) (func(), map[int]types.HotkeyBinding, error) {
	started := make(chan struct {
		threadID uint32
		active   map[int]types.HotkeyBinding
		err      error
	}, 1)
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(doneCh)

		monitorDebug("hotkey worker thread entered")

		active := map[int]types.HotkeyBinding{}
		ids := make([]int, 0, len(bindings))
		failures := make([]string, 0)
		for i, binding := range bindings {
			if !binding.Enabled {
				monitorDebug("skip disabled hotkey", "action", binding.Action, "accelerator", binding.Accelerator)
				continue
			}
			mods, key, ok := parseWindowsAccelerator(binding.Accelerator)
			monitorDebug("parse hotkey", "action", binding.Action, "accelerator", binding.Accelerator, "ok", ok, "mods", fmt.Sprintf("0x%X", mods), "vk", fmt.Sprintf("0x%X", key))
			if !ok {
				failures = append(failures, fmt.Sprintf("%s: invalid accelerator", binding.Action))
				continue
			}
			id := 100 + i
			ret, _, err := procRegisterHotKey.Call(0, uintptr(id), uintptr(mods|modNoRepeat), uintptr(key))
			if ret == 0 {
				reason := err.Error()
				if errno, ok := err.(syscall.Errno); ok && errno == errorHotkeyAlreadyRegistered {
					reason = "快捷键已被系统或其他程序占用"
				}
				failures = append(failures, fmt.Sprintf("%s (%s): %s", binding.Action, binding.Accelerator, reason))
				continue
			}
			ids = append(ids, id)
			active[id] = binding
			monitorInfo("global hotkey registered", "id", id, "accelerator", binding.Accelerator, "action", binding.Action, "mods", fmt.Sprintf("0x%X", mods|modNoRepeat), "vk", fmt.Sprintf("0x%X", key))
		}
		if len(active) == 0 && len(failures) > 0 {
			monitorWarn("hotkey registration failed for all bindings", "failures", strings.Join(failures, "; "))
			started <- struct {
				threadID uint32
				active   map[int]types.HotkeyBinding
				err      error
			}{err: fmt.Errorf("register global hotkeys failed: %s", strings.Join(failures, "; "))}
			return
		}

		threadID := windows.GetCurrentThreadId()
		if len(failures) > 0 {
			monitorWarn("global hotkey partial failures", "failures", strings.Join(failures, "; "))
		}
		started <- struct {
			threadID uint32
			active   map[int]types.HotkeyBinding
			err      error
		}{threadID: threadID, active: active}

		go func() {
			<-stopCh
			monitorDebug("hotkey worker stop requested", "thread", threadID)
			procPostThreadMsg.Call(uintptr(threadID), wmQuit, 0, 0)
		}()
		monitorDebug("global hotkey loop started", "thread", threadID, "active", len(active))

		var message msg
		for {
			ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
			if int32(ret) <= 0 || message.message == wmQuit {
				monitorDebug("global hotkey loop exiting", "thread", threadID)
				unregisterIDs(ids)
				return
			}
			if message.message != wmHotkey {
				continue
			}
			if binding, ok := active[int(message.wParam)]; ok && onAction != nil {
				monitorDebug("WM_HOTKEY matched", "id", int(message.wParam), "accelerator", binding.Accelerator, "action", binding.Action)
				go onAction(binding.Action)
			}
		}
	}()
	result := <-started
	if result.err != nil {
		close(stopCh)
		return nil, nil, result.err
	}
	stop := func() {
		select {
		case <-stopCh:
		default:
			close(stopCh)
		}
		<-doneCh
	}
	return stop, result.active, nil
}

func unregisterIDs(ids []int) {
	for _, id := range ids {
		procUnregister.Call(0, uintptr(id))
	}
}

func parseWindowsAccelerator(acc string) (uint32, uint32, bool) {
	normalized := types.NormalizeAccelerator(acc)
	if normalized == "" {
		return 0, 0, false
	}
	parts := strings.Split(normalized, "+")
	var mods uint32
	var key uint32
	for _, part := range parts {
		switch part {
		case "Ctrl":
			mods |= modCtrl
		case "Alt":
			mods |= modAlt
		case "Shift":
			mods |= modShift
		case "Meta":
			mods |= modWin
		case "Escape":
			key = windows.VK_ESCAPE
		default:
			if len(part) == 1 {
				key = uint32(part[0])
			}
		}
	}
	return mods, key, key != 0
}
