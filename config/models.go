package config

import (
	"fmt"
	"strings"
)

// ActionType определяет тип действия при срабатывании хоткея
type ActionType string

// DefaultVolumeStep — шаг изменения громкости по умолчанию, в процентах.
const DefaultVolumeStep = 10

// Language — язык интерфейса.
type Language string

const (
	LanguageRU Language = "ru"
	LanguageEN Language = "en"
)

const (
	ActionVolumeUp   ActionType = "volume_up"
	ActionVolumeDown ActionType = "volume_down"
	ActionToggleMute ActionType = "toggle_mute"
	ActionPlayPause  ActionType = "play_pause"
	ActionNextTrack  ActionType = "next_track"
	ActionPrevTrack  ActionType = "prev_track"
)

// HotkeyBinding описывает конкретную привязку клавиши к действию.
//
// A8: поле VolumeStep здесь удалено — шаг громкости живёт на уровне профиля
// (ProcessProfile.VolumeStepPercent) и только оттуда читается диспетчером;
// это поле лишь записывалось в Validate и никогда не использовалось, вводя
// в заблуждение (два источника правды для одного шага). Старые config.json
// с "volume_step" у привязки просто игнорируются при разборе — потеря нет.
type HotkeyBinding struct {
	ID        string     `json:"id"`        // Уникальный ID связи
	Action    ActionType `json:"action"`    // Тип действия
	KeyCombo  string     `json:"key_combo"` // Строковое представление ("Ctrl+Alt+Num8")
	KeyCode   uint32     `json:"key_code"`  // Виртуальный код клавиши (VK Code)
	Modifiers uint32     `json:"modifiers"` // Маска модификаторов (MOD_CONTROL, MOD_ALT и т.д.)
}

// ProcessProfile описывает профиль конкретного процесса
type ProcessProfile struct {
	ID          string          `json:"id"`           // Уникальный ID профиля
	ProcessName string          `json:"process_name"` // Имя исполняемого файла (например, "spotify.exe")
	DisplayName string          `json:"display_name"` // Читаемое имя (например, "Spotify")
	Enabled     bool            `json:"enabled"`      // Активен ли профиль
	Hotkeys     []HotkeyBinding `json:"hotkeys"`      // Список хоткеев процесса

	// TargetAppUserModelId — опциональная явная привязка к конкретной media-сессии
	// (SourceAppUserModelId), когда у одного процесса их одновременно несколько
	// (например, несколько вкладок браузера). Пусто = автоопределение по имени
	// процесса (первая подходящая сессия). Влияет только на Play/Pause/Next/Prev —
	// громкость/mute всегда работают по ProcessName напрямую через WASAPI.
	TargetAppUserModelId string `json:"target_app_user_model_id,omitempty"`

	// VolumeStepPercent — на сколько процентов меняется громкость за одно
	// нажатие "громче"/"тише" для ЭТОГО профиля. Хранится на уровне профиля, а
	// не отдельной привязки: в UI это одно поле рядом с парой +/-, и разные
	// значения для "+" и "-" смысла не имеют. 0 = использовать DefaultVolumeStep.
	VolumeStepPercent int `json:"volume_step_percent,omitempty"`
}

// AppConfig — корневой объект конфигурации приложения
type AppConfig struct {
	Version  string           `json:"version"`  // Версия формата конфига
	Profiles []ProcessProfile `json:"profiles"` // Список сохраненных профилей

	// GlobalHotkeysDisabled — общий выключатель. Намеренно хранится в виде "disabled",
	// а не "enabled": у пользователей, уже обновивших приложение, в старом config.json
	// этого поля нет вообще, и JSON-анмаршалинг даст ему нулевое значение false —
	// то есть "не отключено" = хоткеи включены, как и было раньше. Если бы поле
	// называлось GlobalHotkeysEnabled, тот же нулевой false означал бы "выключено",
	// и все существующие пользователи после обновления обнаружили бы, что хоткеи
	// молча перестали работать.
	GlobalHotkeysDisabled bool `json:"global_hotkeys_disabled,omitempty"`

	// Language — язык интерфейса ("ru" / "en"). Пусто = "ru" (значение по
	// умолчанию подставляется в Validate).
	Language Language `json:"language,omitempty"`

	// Autostart — запускать приложение вместе с Windows. Фактическая запись в
	// реестре живёт отдельно (пакет autostart); здесь хранится только желаемое
	// состояние, чтобы UI мог его показать без обращения к реестру.
	Autostart bool `json:"autostart,omitempty"`

	// StartMinimized — открывать приложение сразу свёрнутым в трей, без показа
	// окна. Практически обязателен в паре с Autostart: иначе при каждом входе в
	// Windows пользователю в лицо открывается полноразмерное окно фоновой утилиты.
	StartMinimized bool `json:"start_minimized,omitempty"`
}

// Validate проверяет корректность заполнения данных конфигурации
func (c *AppConfig) Validate() error {
	if c.Version == "" {
		c.Version = "1.0.0"
	}

	if c.Language != LanguageRU && c.Language != LanguageEN {
		c.Language = LanguageRU
	}

	profileIDs := make(map[string]bool)
	for i, profile := range c.Profiles {
		if profile.ProcessName == "" {
			return fmt.Errorf("profile at index %d has empty process_name", i)
		}

		c.Profiles[i].ProcessName = strings.ToLower(profile.ProcessName)

		if profile.ID == "" {
			return fmt.Errorf("profile for %s missing ID", profile.ProcessName)
		}
		if profileIDs[profile.ID] {
			return fmt.Errorf("duplicate profile ID: %s", profile.ID)
		}
		profileIDs[profile.ID] = true

		if profile.VolumeStepPercent != 0 {
			if profile.VolumeStepPercent < 1 {
				c.Profiles[i].VolumeStepPercent = 1
			} else if profile.VolumeStepPercent > 100 {
				c.Profiles[i].VolumeStepPercent = 100
			}
		}

		for j, hk := range profile.Hotkeys {
			if hk.Action == "" {
				return fmt.Errorf("hotkey at index %d in profile %s has no action", j, profile.ProcessName)
			}
		}
	}

	return nil
}
