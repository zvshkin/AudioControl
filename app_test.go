package main

import (
	"testing"

	"audio-control/config"
)

// hotkeyRelevantEqual — фильтр, решающий, нужна ли перерегистрация Win32
// после сохранения профиля. Регресс = два крайних бага: (1) лишний reload на
// каждое нажатие клавиши в поле имени (исторический перформанс-баг) или
// (2) не замеченное изменение комбинации — хоткей молча не перерегистрируется.
func TestHotkeyRelevantEqual(t *testing.T) {
	base := config.ProcessProfile{
		ID:                   "p1",
		ProcessName:          "spotify.exe",
		DisplayName:          "Spotify",
		Enabled:              true,
		TargetAppUserModelId: "",
		VolumeStepPercent:    10,
		Hotkeys: []config.HotkeyBinding{
			{ID: "h1", Action: config.ActionVolumeUp, KeyCode: 0x61, Modifiers: 0x2},
		},
	}

	same := base
	if !hotkeyRelevantEqual(base, same) {
		t.Error("идентичные профили обязаны считаться равными — иначе перерегистрация на каждом сохранении")
	}

	renamed := base
	renamed.DisplayName = "Spotify Music"
	if !hotkeyRelevantEqual(base, renamed) {
		t.Error("переименование не должно требовать перерегистрации")
	}

	relevantChanges := map[string]func(*config.ProcessProfile){
		"другой процесс":       func(p *config.ProcessProfile) { p.ProcessName = "chrome.exe" },
		"вкл/выкл профиля":     func(p *config.ProcessProfile) { p.Enabled = false },
		"другая media-цель":    func(p *config.ProcessProfile) { p.TargetAppUserModelId = "app!id" },
		"другой шаг громкости": func(p *config.ProcessProfile) { p.VolumeStepPercent = 5 },
		"другая комбинация": func(p *config.ProcessProfile) {
			p.Hotkeys = []config.HotkeyBinding{
				{ID: "h1", Action: config.ActionVolumeUp, KeyCode: 0x62, Modifiers: 0x2},
			}
		},
	}

	for name, mutate := range relevantChanges {
		t.Run(name, func(t *testing.T) {
			modified := base
			mutate(&modified)
			if hotkeyRelevantEqual(base, modified) {
				t.Errorf("%q: изменение должно запускать перерегистрацию, а равные профили отданы", name)
			}
		})
	}
}
