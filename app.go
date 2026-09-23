package main

import (
	"context"
	"fmt"
	"slices"

	"audio-control/audio"
	"audio-control/autostart"
	"audio-control/config"
	"audio-control/hotkey"
	"audio-control/media"
	"audio-control/process"
	"audio-control/tray"

	"github.com/google/uuid"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// trayLabels возвращает подписи меню трея на выбранном языке.
func trayLabels(lang config.Language) tray.Labels {
	if lang == config.LanguageEN {
		return tray.Labels{
			Show:           "Show",
			HotkeysEnabled: "Global hotkeys",
			Exit:           "Exit",
		}
	}
	return tray.Labels{
		Show:           "Показать",
		HotkeysEnabled: "Горячие клавиши",
		Exit:           "Выход",
	}
}

type App struct {
	ctx context.Context

	cfgMgr     *config.Manager
	audioMgr   *audio.AudioManager
	mediaMgr   *media.Manager
	scanner    *process.ProcessScanner
	dispatcher *hotkey.Dispatcher
	systray    *tray.Tray
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	cfgMgr, err := config.NewManager()
	if err != nil {
		appLog.Println("config.NewManager error:", err)
		return
	}
	a.cfgMgr = cfgMgr

	if _, err := a.cfgMgr.Load(); err != nil {
		appLog.Println("config Load error:", err)
	}

	a.audioMgr = audio.NewAudioManager()
	a.mediaMgr = media.NewManager()
	a.scanner = process.NewScanner()

	a.dispatcher = hotkey.NewDispatcher(a.cfgMgr, a.audioMgr, a.mediaMgr)

	a.dispatcher.SetMediaStatusCallback(func(profileID, processName string, action config.ActionType, available bool) {
		wailsRuntime.EventsEmit(a.ctx, "media:target-status", map[string]interface{}{
			"profileId":   profileID,
			"processName": processName,
			"action":      string(action),
			"available":   available,
		})
	})

	a.dispatcher.SetActionErrorCallback(func(profileName string, action config.ActionType, message string) {
		wailsRuntime.EventsEmit(a.ctx, "hotkey:action-error", map[string]interface{}{
			"profileName": profileName,
			"action":      string(action),
			"message":     message,
		})
	})

	cfg := a.cfgMgr.Get()

	if err := autostart.Sync(cfg.Autostart); err != nil {
		appLog.Println("autostart sync error:", err)
	}

	if cfg.GlobalHotkeysDisabled {
		_ = a.dispatcher.SetGlobalEnabled(false)
	}

	if err := a.dispatcher.Start(); err != nil {
		appLog.Println("hotkey dispatcher start error:", err)
	}

	a.systray = tray.New(tray.Options{
		Tooltip:   "AudioControl Central",
		IconBytes: trayIconICO,
		Labels:    trayLabels(cfg.Language),
		OnShow: func() {
			wailsRuntime.WindowShow(a.ctx)
			wailsRuntime.WindowUnminimise(a.ctx)
		},
		OnExit: func() {
			wailsRuntime.Quit(a.ctx)
		},
		OnToggleHotkeys: func() bool {
			current := a.cfgMgr.Get()
			newEnabled := current.GlobalHotkeysDisabled

			_ = a.cfgMgr.Update(func(c *config.AppConfig) error {
				c.GlobalHotkeysDisabled = !newEnabled
				return nil
			})
			_ = a.dispatcher.SetGlobalEnabled(newEnabled)

			wailsRuntime.EventsEmit(a.ctx, "hotkeys:global-toggled", newEnabled)
			return newEnabled
		},
		HotkeysEnabled: !cfg.GlobalHotkeysDisabled,
	})
	if err := a.systray.Start(); err != nil {

		appLog.Println("tray start error:", err)
		showStartupError("AudioControl Central",
			"Не удалось запустить иконку в трее: "+err.Error()+
				"\nСворачивание в трей будет недоступно.")
	}
}

func (a *App) shutdown(ctx context.Context) {
	if a.dispatcher != nil {
		a.dispatcher.Stop()
	}
	if a.mediaMgr != nil {
		a.mediaMgr.Stop()
	}
	if a.systray != nil {
		a.systray.Stop()
	}
}

func (a *App) GetConfig() (config.AppConfig, error) {
	if a.cfgMgr == nil {
		return config.AppConfig{}, fmt.Errorf("config manager is not initialized")
	}

	if !a.cfgMgr.Loaded() {
		return config.AppConfig{}, fmt.Errorf("config.json could not be loaded; original kept at config.json.bak")
	}
	return a.cfgMgr.Get(), nil
}

func (a *App) GetRunningProcesses() ([]process.ProcessInfo, error) {
	if a.scanner == nil {
		return nil, fmt.Errorf("process scanner is not initialized")
	}
	return a.scanner.GetActiveProcesses()
}

// hotkeyRelevantEqual сообщает, отличаются ли два состояния профиля только в
// частях, НЕ влияющих на RegisterHotKey и диспетчеризацию. DisplayName
// намеренно вне сравнения: переименование профиля не должно снимать и
// переставлять все комбинации — раньше SaveProfile звал reloadHotkeys() на
// ЛЮБОЕ изменение, включая правку имени (даже посимвольную: запись конфига
// на каждое нажатие клавиши + полный unregister/register всех хоткеев).
// Актуальное имя для тостов и диагностики подтягивает
// Dispatcher.currentProfileName / LastRegistrationStatus.
func hotkeyRelevantEqual(a, b config.ProcessProfile) bool {
	return a.Enabled == b.Enabled &&
		a.ProcessName == b.ProcessName &&
		a.TargetAppUserModelId == b.TargetAppUserModelId &&
		a.VolumeStepPercent == b.VolumeStepPercent &&
		slices.Equal(a.Hotkeys, b.Hotkeys)
}

func (a *App) SaveProfile(profile config.ProcessProfile) (config.AppConfig, error) {
	if a.cfgMgr == nil {
		return config.AppConfig{}, fmt.Errorf("config manager is not initialized")
	}

	if profile.ID == "" {
		profile.ID = uuid.NewString()
	}

	needsReload := false
	err := a.cfgMgr.Update(func(cfg *config.AppConfig) error {
		for i, p := range cfg.Profiles {
			if p.ID == profile.ID {
				needsReload = !hotkeyRelevantEqual(p, profile)
				cfg.Profiles[i] = profile
				return nil
			}
		}

		for _, hk := range profile.Hotkeys {
			if hk.KeyCode != 0 {
				needsReload = true
				break
			}
		}
		cfg.Profiles = append(cfg.Profiles, profile)
		return nil
	})
	if err != nil {
		return config.AppConfig{}, err
	}

	if needsReload {
		a.reloadHotkeys()
	}
	return a.cfgMgr.Get(), nil
}

func (a *App) DeleteProfile(id string) (config.AppConfig, error) {
	if a.cfgMgr == nil {
		return config.AppConfig{}, fmt.Errorf("config manager is not initialized")
	}

	err := a.cfgMgr.Update(func(cfg *config.AppConfig) error {
		filtered := make([]config.ProcessProfile, 0, len(cfg.Profiles))
		for _, p := range cfg.Profiles {
			if p.ID != id {
				filtered = append(filtered, p)
			}
		}
		cfg.Profiles = filtered
		return nil
	})
	if err != nil {
		return config.AppConfig{}, err
	}

	a.reloadHotkeys()
	return a.cfgMgr.Get(), nil
}

func (a *App) GetHotkeyConflicts() ([]hotkey.Conflict, error) {
	if a.cfgMgr == nil {
		return nil, fmt.Errorf("config manager is not initialized")
	}
	return hotkey.FindConflicts(a.cfgMgr.Get()), nil
}

// GetHotkeyStatus возвращает фактический результат регистрации каждой комбинации:
// какие хоткеи реально живы, а какие Windows отклонила (обычно потому, что
// комбинация уже занята другой программой). Это главный инструмент диагностики
// ситуации "нажимаю — ничего не происходит".
func (a *App) GetHotkeyStatus() ([]hotkey.RegistrationStatus, error) {
	if a.dispatcher == nil {
		return nil, fmt.Errorf("dispatcher is not initialized")
	}
	return a.dispatcher.LastRegistrationStatus(), nil
}

// TestAction выполняет действие профиля прямо сейчас, без нажатия хоткея —
// позволяет отделить проблему "хоткей не ловится" от проблемы "звук не меняется".
func (a *App) TestAction(profileID string, action config.ActionType) (string, error) {
	if a.cfgMgr == nil {
		return "", fmt.Errorf("config manager is not initialized")
	}

	cfg := a.cfgMgr.Get()
	for _, p := range cfg.Profiles {
		if p.ID != profileID {
			continue
		}

		step := p.VolumeStepPercent
		if step <= 0 {
			step = config.DefaultVolumeStep
		}

		switch action {
		case config.ActionVolumeUp:
			vol, err := a.audioMgr.ChangeVolume(0, p.ProcessName, step)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d%%", vol), nil

		case config.ActionVolumeDown:
			vol, err := a.audioMgr.ChangeVolume(0, p.ProcessName, -step)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d%%", vol), nil

		case config.ActionToggleMute:
			muted, err := a.audioMgr.ToggleMute(0, p.ProcessName)
			if err != nil {
				return "", err
			}
			if muted {
				return "muted", nil
			}
			return "unmuted", nil

		default:
			return "", fmt.Errorf("unsupported test action: %s", action)
		}
	}

	return "", fmt.Errorf("profile not found: %s", profileID)
}

func (a *App) GetMediaTargets(processName string) ([]string, error) {
	if a.mediaMgr == nil {
		return nil, fmt.Errorf("media manager is not initialized")
	}
	return a.mediaMgr.ListTargets(processName)
}

// GetMediaTargetsBatch — батч-вариант GetMediaTargets: цели для всех
// медиа-профилей за один WinRT-обход вместо отдельного SessionManager на
// каждый процесс (см. media.ListTargetsBatch).
func (a *App) GetMediaTargetsBatch(processNames []string) (map[string][]string, error) {
	if a.mediaMgr == nil {
		return nil, fmt.Errorf("media manager is not initialized")
	}
	return a.mediaMgr.ListTargetsBatch(processNames)
}

func (a *App) SetGlobalHotkeysEnabled(enabled bool) (config.AppConfig, error) {
	if a.cfgMgr == nil {
		return config.AppConfig{}, fmt.Errorf("config manager is not initialized")
	}

	err := a.cfgMgr.Update(func(cfg *config.AppConfig) error {
		cfg.GlobalHotkeysDisabled = !enabled
		return nil
	})
	if err != nil {
		return config.AppConfig{}, err
	}

	if a.dispatcher != nil {
		if err := a.dispatcher.SetGlobalEnabled(enabled); err != nil {
			appLog.Println("SetGlobalEnabled error:", err)
		}
	}

	if a.systray != nil {
		a.systray.SetHotkeysEnabled(enabled)
	}

	return a.cfgMgr.Get(), nil
}

// SetLanguage меняет язык интерфейса (и подписи в меню трея).
func (a *App) SetLanguage(lang string) (config.AppConfig, error) {
	if a.cfgMgr == nil {
		return config.AppConfig{}, fmt.Errorf("config manager is not initialized")
	}

	target := config.Language(lang)
	if target != config.LanguageRU && target != config.LanguageEN {
		return config.AppConfig{}, fmt.Errorf("unsupported language: %s", lang)
	}

	err := a.cfgMgr.Update(func(cfg *config.AppConfig) error {
		cfg.Language = target
		return nil
	})
	if err != nil {
		return config.AppConfig{}, err
	}

	if a.systray != nil {
		a.systray.SetLabels(trayLabels(target))
	}

	return a.cfgMgr.Get(), nil
}

// SetAutostart включает/выключает запуск вместе с Windows.
func (a *App) SetAutostart(enabled bool) (config.AppConfig, error) {
	if a.cfgMgr == nil {
		return config.AppConfig{}, fmt.Errorf("config manager is not initialized")
	}

	if enabled {
		if err := autostart.Enable(); err != nil {
			return config.AppConfig{}, err
		}
	} else {
		if err := autostart.Disable(); err != nil {
			return config.AppConfig{}, err
		}
	}

	err := a.cfgMgr.Update(func(cfg *config.AppConfig) error {
		cfg.Autostart = enabled
		return nil
	})
	if err != nil {
		return config.AppConfig{}, err
	}

	return a.cfgMgr.Get(), nil
}

// IsAutostartEnabled читает фактическое состояние из реестра (оно могло быть
// изменено извне — например, через диспетчер задач).
func (a *App) IsAutostartEnabled() bool {
	return autostart.IsEnabled()
}

// ProfileState — то, что интерфейс показывает рядом с профилем: реальная
// текущая громкость процесса, состояние mute и признак того, что процесс вообще
// сейчас звучит. Раньше интерфейс был "слепым": пользователь жал хоткей и не
// видел результата, не открыв микшер Windows.
type ProfileState struct {
	ProfileID  string `json:"profile_id"`
	HasSession bool   `json:"has_session"`
	Volume     int    `json:"volume"`
	Muted      bool   `json:"muted"`
	Error      string `json:"error,omitempty"`
}

// GetProfileStates возвращает состояние сразу для всех профилей одним вызовом:
// раньше каждый профиль поднимал своё COM-окружение (CoInitialize,
// MMDeviceEnumerator, Activate, GetSessionEnumerator) — при N профилях и
// поллинге каждые 3 с это было N полных обходов на тик. Теперь один обход
// на все профили (см. audio.GetVolumesBatch).
func (a *App) GetProfileStates() ([]ProfileState, error) {
	if a.cfgMgr == nil {
		return nil, fmt.Errorf("config manager is not initialized")
	}

	cfg := a.cfgMgr.Get()
	states := make([]ProfileState, 0, len(cfg.Profiles))

	names := make([]string, 0, len(cfg.Profiles))
	for _, p := range cfg.Profiles {
		if p.ProcessName != "" {
			names = append(names, p.ProcessName)
		}
	}
	volumes := a.audioMgr.GetVolumesBatch(names)

	for _, p := range cfg.Profiles {
		st := ProfileState{ProfileID: p.ID}

		if p.ProcessName == "" {
			states = append(states, st)
			continue
		}

		sv, ok := volumes[p.ProcessName]
		if !ok || !sv.Found {

			st.HasSession = false
			states = append(states, st)
			continue
		}

		st.HasSession = true
		st.Volume = sv.Volume
		st.Muted = sv.Muted
		states = append(states, st)
	}

	return states, nil
}

// SetProfileVolume выставляет абсолютную громкость процесса профиля (слайдер в UI).
func (a *App) SetProfileVolume(profileID string, percent int) (ProfileState, error) {
	profile, err := a.findProfile(profileID)
	if err != nil {
		return ProfileState{}, err
	}

	vol, err := a.audioMgr.SetVolume(0, profile.ProcessName, percent)
	if err != nil {
		return ProfileState{}, err
	}

	return ProfileState{ProfileID: profileID, HasSession: true, Volume: vol}, nil
}

// SetProfileMute выставляет конкретное состояние mute (кнопка в UI).
func (a *App) SetProfileMute(profileID string, muted bool) (ProfileState, error) {
	profile, err := a.findProfile(profileID)
	if err != nil {
		return ProfileState{}, err
	}

	if err := a.audioMgr.SetMute(0, profile.ProcessName, muted); err != nil {
		return ProfileState{}, err
	}

	vol, isMuted, err := a.audioMgr.GetVolume(0, profile.ProcessName)
	if err != nil {
		return ProfileState{ProfileID: profileID, HasSession: true, Muted: muted}, nil
	}

	return ProfileState{ProfileID: profileID, HasSession: true, Volume: vol, Muted: isMuted}, nil
}

// DuplicateProfile создаёт копию профиля вместе со всеми настройками, но БЕЗ
// хоткеев: копировать их означало бы гарантированно создать конфликт комбинаций,
// а новые всё равно пришлось бы назначать заново.
func (a *App) DuplicateProfile(profileID string) (config.AppConfig, error) {
	if a.cfgMgr == nil {
		return config.AppConfig{}, fmt.Errorf("config manager is not initialized")
	}

	err := a.cfgMgr.Update(func(cfg *config.AppConfig) error {
		for _, p := range cfg.Profiles {
			if p.ID != profileID {
				continue
			}

			copied := p
			copied.ID = uuid.NewString()
			copied.DisplayName = p.DisplayName + " (copy)"
			copied.Hotkeys = []config.HotkeyBinding{}

			cfg.Profiles = append(cfg.Profiles, copied)
			return nil
		}
		return fmt.Errorf("profile not found: %s", profileID)
	})
	if err != nil {
		return config.AppConfig{}, err
	}

	a.reloadHotkeys()
	return a.cfgMgr.Get(), nil
}

// MoveProfile сдвигает профиль в списке на одну позицию (delta -1 или +1).
func (a *App) MoveProfile(profileID string, delta int) (config.AppConfig, error) {
	if a.cfgMgr == nil {
		return config.AppConfig{}, fmt.Errorf("config manager is not initialized")
	}

	err := a.cfgMgr.Update(func(cfg *config.AppConfig) error {
		idx := -1
		for i, p := range cfg.Profiles {
			if p.ID == profileID {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fmt.Errorf("profile not found: %s", profileID)
		}

		target := idx + delta
		if target < 0 || target >= len(cfg.Profiles) {
			return nil
		}

		cfg.Profiles[idx], cfg.Profiles[target] = cfg.Profiles[target], cfg.Profiles[idx]
		return nil
	})
	if err != nil {
		return config.AppConfig{}, err
	}

	return a.cfgMgr.Get(), nil
}

// SetStartMinimized включает/выключает запуск свёрнутым в трей.
func (a *App) SetStartMinimized(enabled bool) (config.AppConfig, error) {
	if a.cfgMgr == nil {
		return config.AppConfig{}, fmt.Errorf("config manager is not initialized")
	}

	err := a.cfgMgr.Update(func(cfg *config.AppConfig) error {
		cfg.StartMinimized = enabled
		return nil
	})
	if err != nil {
		return config.AppConfig{}, err
	}

	return a.cfgMgr.Get(), nil
}

func (a *App) findProfile(profileID string) (config.ProcessProfile, error) {
	if a.cfgMgr == nil {
		return config.ProcessProfile{}, fmt.Errorf("config manager is not initialized")
	}

	cfg := a.cfgMgr.Get()
	for _, p := range cfg.Profiles {
		if p.ID == profileID {
			return p, nil
		}
	}

	return config.ProcessProfile{}, fmt.Errorf("profile not found: %s", profileID)
}

func (a *App) HideToTray() {
	wailsRuntime.WindowHide(a.ctx)
}

func (a *App) MinimizeWindow() {
	wailsRuntime.WindowMinimise(a.ctx)
}

func (a *App) reloadHotkeys() {
	if a.dispatcher == nil {
		return
	}
	if err := a.dispatcher.Reload(); err != nil {

		wailsRuntime.EventsEmit(a.ctx, "hotkeys:registration-issue", err.Error())
	}
}
