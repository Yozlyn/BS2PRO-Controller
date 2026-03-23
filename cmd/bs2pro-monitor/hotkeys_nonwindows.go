//go:build !windows

package main

import "github.com/TIANLI0/BS2PRO-Controller/internal/types"

func registerGlobalHotkeys(bindings []types.HotkeyBinding, onAction func(string)) (func(), map[int]types.HotkeyBinding, error) {
	return func() {}, map[int]types.HotkeyBinding{}, nil
}
