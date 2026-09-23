package hotkey

import (
	"testing"

	"audio-control/config"
)

// FindConflicts — чистая функция без Win32: именно её результат UI показывает
// как «комбинация занята самим собой». Регресс здесь означает либо ложные
// конфликты, либо невидимые настоящие.
func TestFindConflicts(t *testing.T) {
	cfg := config.AppConfig{
		Profiles: []config.ProcessProfile{
			{
				ID: "p1", ProcessName: "a.exe", DisplayName: "A",
				Hotkeys: []config.HotkeyBinding{{ID: "h1", Action: config.ActionVolumeUp, KeyCode: 0x61, Modifiers: 0x2}},
			},
			{
				ID: "p2", ProcessName: "b.exe", DisplayName: "B",
				Hotkeys: []config.HotkeyBinding{
					{ID: "h2", Action: config.ActionVolumeDown, KeyCode: 0x61, Modifiers: 0x2},
					{ID: "h3", Action: config.ActionToggleMute, KeyCode: 0x62, Modifiers: 0x2},
				},
			},
			{
				ID: "p3", ProcessName: "c.exe", DisplayName: "C",

				Hotkeys: []config.HotkeyBinding{{ID: "h4", Action: config.ActionVolumeUp}},
			},
		},
	}

	conflicts := FindConflicts(cfg)
	if len(conflicts) != 1 {
		t.Fatalf("FindConflicts() = %d конфликтов, хотели ровно 1: %+v", len(conflicts), conflicts)
	}

	c := conflicts[0]
	if c.KeyCode != 0x61 || c.Modifiers != 0x2 {
		t.Errorf("конфликт на %+v, ожидали Ctrl+A", c)
	}
	if len(c.Bindings) != 2 {
		t.Errorf("в конфликте %d привязок, ожидали 2", len(c.Bindings))
	}

	seen := map[string]bool{}
	for _, b := range c.Bindings {
		seen[b.ProfileID] = true
	}
	if !seen["p1"] || !seen["p2"] {
		t.Errorf("участники конфликта %v, ожидали p1 и p2", seen)
	}
}

func TestFindConflictsNoneWhenAllDistinct(t *testing.T) {
	cfg := config.AppConfig{
		Profiles: []config.ProcessProfile{
			{ID: "p1", Hotkeys: []config.HotkeyBinding{{KeyCode: 0x61, Modifiers: 0x2}}},
			{ID: "p2", Hotkeys: []config.HotkeyBinding{{KeyCode: 0x62, Modifiers: 0x2}}},
			{ID: "p3", Hotkeys: []config.HotkeyBinding{{KeyCode: 0x61, Modifiers: 0x4}}},
		},
	}

	if got := FindConflicts(cfg); len(got) != 0 {
		t.Errorf("FindConflicts() = %d конфликтов, ожидали 0: %+v", len(got), got)
	}
}
