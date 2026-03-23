package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/TIANLI0/BS2PRO-Controller/internal/config"
)

type ThemePreference struct {
	FollowSystem bool   `json:"followSystem"`
	Mode         string `json:"mode"`
}

func defaultThemePreference() ThemePreference {
	return ThemePreference{FollowSystem: true, Mode: "light"}
}

func themePreferencePath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(config.GetInstallDir(), "theme-preference.json")
	}
	return filepath.Join(configDir, "BS2PRO-Controller", "theme-preference.json")
}

func loadThemePreference() ThemePreference {
	path := themePreferencePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultThemePreference()
	}

	pref := defaultThemePreference()
	if err := json.Unmarshal(data, &pref); err != nil {
		return defaultThemePreference()
	}
	if pref.Mode != "dark" && pref.Mode != "light" {
		pref.Mode = "light"
	}
	return pref
}

func saveThemePreference(pref ThemePreference) error {
	if pref.Mode != "dark" && pref.Mode != "light" {
		pref.Mode = "light"
	}
	path := themePreferencePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(pref)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
