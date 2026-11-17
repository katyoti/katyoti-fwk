// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/katyoti/katyoti-fwk/app/ktw-gui/internal/settings"
)

// GetGuiSettings returns the persisted GUI settings from disk
// (~/.config/katyoti/config/GuiSettings.yaml). On first run or any read
// error, returns the baseline DEFAULT settings — never errors back to JS,
// since the frontend always wants something to render.
//
// Wails-bound: the JS side calls window.go.main.App.GetGuiSettings().
func (a *App) GetGuiSettings() settings.GuiSettings {
	s, err := settings.Load()
	if err != nil {
		// Log but don't surface — frontend uses defaults transparently.
		// Future: emit a Wails event so the GUI can show a toast.
		_ = err
	}
	return s
}

// SaveGuiSettings persists the GUI settings to disk. Called by the frontend
// after any change in the settings dialog (debounced ~300ms in zustand).
//
// Returns the error string (empty on success) so the frontend can show a
// toast on failure.
//
// Wails-bound: the JS side calls window.go.main.App.SaveGuiSettings(obj).
func (a *App) SaveGuiSettings(s settings.GuiSettings) string {
	if err := settings.Save(s); err != nil {
		return err.Error()
	}
	return ""
}

// GetGuiSettingsPath returns the absolute path of the GuiSettings.yaml file
// — surfaced in the settings dialog footer so users know where their config
// lives ("Stored at: …").
func (a *App) GetGuiSettingsPath() string {
	return settings.SettingsPath()
}
