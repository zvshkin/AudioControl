package config

import (
	"strings"
	"testing"
)

// validConfig — минимальный конфиг, проходящий Validate без изменений.
func validConfig() *AppConfig {
	return &AppConfig{
		Version: "1.0.0",
		Profiles: []ProcessProfile{
			{
				ID:          "p1",
				ProcessName: "spotify.exe",
				DisplayName: "Spotify",
				Enabled:     true,
				Hotkeys: []HotkeyBinding{
					{ID: "h1", Action: ActionVolumeUp, KeyCode: 0x61, Modifiers: 0x0002},
				},
			},
		},
		Language: LanguageRU,
	}
}

// Validate должен заполнять дефолты для «старых» конфигов, в которых полей
// ещё не существовало: иначе обновление приложения ломает пользовательский
// config.json (исторический баг: отсутствие GlobalHotkeysDisabled в старом
// JSON означало «хоткеи выключены» — см. комментарий к AppConfig).
func TestValidateFillsDefaults(t *testing.T) {
	cfg := &AppConfig{Profiles: []ProcessProfile{}}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if cfg.Version == "" {
		t.Error("Version не заполнен — старый конфиг остался без версии")
	}
	if cfg.Language != LanguageRU {
		t.Errorf("Language = %q, хотели %q", cfg.Language, LanguageRU)
	}
}

func TestValidateNormalizesProcessNameToLower(t *testing.T) {
	cfg := validConfig()
	cfg.Profiles[0].ProcessName = "Spotify.EXE"

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got := cfg.Profiles[0].ProcessName; got != "spotify.exe" {
		t.Errorf("ProcessName = %q, хотели нижний регистр", got)
	}
}

func TestValidateRejectsEmptyProcessName(t *testing.T) {
	cfg := validConfig()
	cfg.Profiles[0].ProcessName = ""

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() обязан отклонять профиль без process_name")
	}
	if !strings.Contains(err.Error(), "process_name") {
		t.Errorf("ошибка %q не упоминает process_name", err)
	}
}

func TestValidateRejectsDuplicateProfileIDs(t *testing.T) {
	cfg := validConfig()
	dup := cfg.Profiles[0]
	cfg.Profiles = append(cfg.Profiles, dup)

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() обязан ловить дубликаты ID профилей")
	}
}

func TestValidateRejectsMissingProfileID(t *testing.T) {
	cfg := validConfig()
	cfg.Profiles[0].ID = ""

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() обязан отклонять профиль без ID")
	}
}

func TestValidateRejectsHotkeyWithoutAction(t *testing.T) {
	cfg := validConfig()
	cfg.Profiles[0].Hotkeys[0].Action = ""

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() обязан отклонять хоткей без действия")
	}
}

// Шаг громкости зажимается в 1..100: из UI нельзя задать отрицательный или
// бессмысленно большой шаг — иначе хоткей мгновенно уводил бы громкость в 0.
func TestValidateClampsVolumeStepPercent(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{-5, 1},
		{0, 0},
		{10, 10},
		{500, 100},
	}

	for _, tc := range cases {
		cfg := validConfig()
		cfg.Profiles[0].VolumeStepPercent = tc.in
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate(in=%d) error = %v", tc.in, err)
		}
		if got := cfg.Profiles[0].VolumeStepPercent; got != tc.want {
			t.Errorf("VolumeStepPercent(in=%d) = %d, хотели %d", tc.in, got, tc.want)
		}
	}
}
