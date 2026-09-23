package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	ConfigDirName  = "AudioControl"
	ConfigFileName = "config.json"
	CurrentVersion = "1.0.0"
)

type Manager struct {
	mu         sync.RWMutex
	configPath string
	current    *AppConfig

	// loadFailed — содержимое config.json ещё ни разу не удалось разобрать.
	// Пока флаг поднят, saveLocked отказывается перезаписывать файл: иначе
	// сценарий "JSON испорчен при ручной правке" превращался в тихую полную
	// потерю профилей — Load падает, менеджер живёт на дефолтах, а первое же
	// Update стирает пользовательский файл своими нулями. Флаг поднимается
	// в NewManager и в ошибках Load и снимается ТОЛЬКО после успешного чтения.
	loadFailed bool
}

// NewManager создает менеджер и инициализирует путь к %APPDATA%
func NewManager() (*Manager, error) {
	appDataDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get AppData directory: %w", err)
	}

	targetDir := filepath.Join(appDataDir, ConfigDirName)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create app config directory: %w", err)
	}

	configPath := filepath.Join(targetDir, ConfigFileName)

	mgr := &Manager{
		configPath: configPath,
		current:    defaultConfig(),

		loadFailed: true,
	}

	return mgr, nil
}

// Load загружает конфигурацию с диска. Если файла нет — создает дефолтный.
func (m *Manager) Load() (*AppConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, err := os.Stat(m.configPath); errors.Is(err, os.ErrNotExist) {

		m.current = defaultConfig()
		m.loadFailed = false
		if err := m.saveLocked(m.current); err != nil {
			return nil, fmt.Errorf("failed to save initial config: %w", err)
		}
		return m.current, nil
	}

	data, err := os.ReadFile(m.configPath)
	if err != nil {

		m.loadFailed = true
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		m.markBrokenLocked(data)
		return nil, fmt.Errorf("failed to parse config JSON (original kept at %s.bak): %w", m.configPath, err)
	}

	if err := cfg.Validate(); err != nil {
		m.markBrokenLocked(data)
		return nil, fmt.Errorf("config validation failed (original kept at %s.bak): %w", m.configPath, err)
	}

	m.current = &cfg
	m.loadFailed = false
	return m.current, nil
}

// Loaded сообщает, удалось ли прочитать и разобрать конфиг с диска. Пока он
// false, данные в менеджере — дефолты, а не пользовательские: сохраняться они
// не дадут (см. saveLocked), а UI лучше честно показать ошибку, чем «чистый»
// пустой список профилей. Вызывать после Load().
func (m *Manager) Loaded() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return !m.loadFailed
}

// markBrokenLocked копирует неразобранный оригинал рядом (config.json.bak),
// чтобы его можно было восстановить вручную, и запрещает дальнейшие
// сохранения до успешного Load(). Вызывается строго под m.mu.
//
// Зачем: без этой защиты сценарий «пользователь поправил JSON руками и
// испортил его» означал потерю ВСЕХ профилей — Load падал, менеджер оставался
// на дефолтах, а первое же Update первым делом перезаписывал файл.
func (m *Manager) markBrokenLocked(data []byte) {
	m.loadFailed = true
	_ = os.WriteFile(m.configPath+".bak", data, 0644)
}

// Get возвращает ГЛУБОКУЮ копию текущего конфига. Поверхностного
// struct-присваивания недостаточно: слайс Profiles и вложенные Hotkeys
// остались бы общими с внутренним состоянием, и любая мутация через результат
// Get() меняла бы конфиг в обход Update/Validate — «потокобезопасная копия»
// была бы фикцией, а не защитой.
func (m *Manager) Get() AppConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return *deepCopyConfig(m.current)
}

// Update выполняет транзакционное изменение конфигурации и автосохранение на диск
func (m *Manager) Update(fn func(cfg *AppConfig) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	updatedCfg := deepCopyConfig(m.current)

	if err := fn(updatedCfg); err != nil {
		return fmt.Errorf("update function error: %w", err)
	}

	if err := updatedCfg.Validate(); err != nil {
		return fmt.Errorf("updated config validation failed: %w", err)
	}

	if err := m.saveLocked(updatedCfg); err != nil {
		return err
	}
	m.current = updatedCfg
	return nil
}

func deepCopyConfig(src *AppConfig) *AppConfig {
	dst := &AppConfig{
		Version:               src.Version,
		Profiles:              make([]ProcessProfile, len(src.Profiles)),
		GlobalHotkeysDisabled: src.GlobalHotkeysDisabled,
		Language:              src.Language,
		Autostart:             src.Autostart,
		StartMinimized:        src.StartMinimized,
	}

	for i, p := range src.Profiles {
		hotkeysCopy := make([]HotkeyBinding, len(p.Hotkeys))
		copy(hotkeysCopy, p.Hotkeys)

		dst.Profiles[i] = ProcessProfile{
			ID:                   p.ID,
			ProcessName:          p.ProcessName,
			DisplayName:          p.DisplayName,
			Enabled:              p.Enabled,
			Hotkeys:              hotkeysCopy,
			TargetAppUserModelId: p.TargetAppUserModelId,
			VolumeStepPercent:    p.VolumeStepPercent,
		}
	}

	return dst
}

// saveLocked записывает cfg на диск (должен вызываться под m.mu.Lock).
// Состояние m.current НЕ меняет — коммит в памяти делает вызывающий и всегда
// после успешной записи (см. Update): так сбой записи не оставляет расхождения
// между памятью и файлом.
func (m *Manager) saveLocked(cfg *AppConfig) error {

	if m.loadFailed {
		return fmt.Errorf("config file was never loaded successfully; refusing to overwrite it (original kept at %s.bak)", m.configPath)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	tempFile := m.configPath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp config file: %w", err)
	}

	if err := os.Rename(tempFile, m.configPath); err != nil {
		return fmt.Errorf("failed to replace config file: %w", err)
	}

	return nil
}

func defaultConfig() *AppConfig {
	return &AppConfig{
		Version:  CurrentVersion,
		Profiles: []ProcessProfile{},
		Language: LanguageRU,
	}
}
