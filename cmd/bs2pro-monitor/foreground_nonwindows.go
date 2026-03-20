//go:build !windows

package main

func getForegroundProcessName() string {
	return ""
}
