package config

import (
	"os"
	"path/filepath"
	"testing"
)

// newTestManager создаёт менеджер, работающий во временном каталоге теста, в
// состоянии «конфиг ещё не загружен» — ровно как после NewManager(). Прямая
// инициализация вместо NewManager() нужна, чтобы не трогать реальный
// %AppData%\AudioControl пользователя, который NewManager() берёт напрямую
// из os.UserConfigDir().
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	return &Manager{
		configPath: filepath.Join(t.TempDir(), ConfigFileName),
		current:    defaultConfig(),
		loadFailed: true,
	}
}

// Get обязан отдавать ГЛУБОКУЮ копию: поверхностное копирование struct
// оставило бы слайс Profiles общим с внутренним состоянием, и мутация через
// результат Get() меняла бы конфиг в обход Update/Validate (баг B7).
func TestGetReturnsDeepCopy(t *testing.T) {
	m := newTestManager(t)
	m.loadFailed = false
	m.current.Profiles = []ProcessProfile{{ID: "p1", ProcessName: "a.exe"}}

	got := m.Get()
	got.Profiles[0].ProcessName = "hacked.exe"
	got.Profiles[0].DisplayName = "hacked"

	again := m.Get()
	if again.Profiles[0].ProcessName != "a.exe" {
		t.Errorf("мутация через Get() просочилась в менеджер: %q", again.Profiles[0].ProcessName)
	}
}

// Update пишет файл ДО коммита в память (баг B9): при сбое записи состояние
// в памяти обязано остаться ровно тем, что лежит на диске — иначе пользователь
// видит «сохранено», а после перезапуска изменения исчезают.
func TestUpdateDoesNotCommitWhenSaveFails(t *testing.T) {
	m := newTestManager(t)
	m.loadFailed = false
	m.configPath = filepath.Join(t.TempDir(), "nonexistent-subdir", "deep", ConfigFileName)

	original := m.Get()

	err := m.Update(func(cfg *AppConfig) error {
		cfg.Language = LanguageEN
		return nil
	})
	if err == nil {
		t.Fatal("Update() обязан вернуть ошибку, когда запись файла невозможна")
	}

	if got := m.Get(); got.Language != original.Language {
		t.Errorf("память изменена при неудачной записи: language = %q, ожидали %q",
			got.Language, original.Language)
	}
}

// При ошибке fn() или Validate() файл и память не трогаются вообще.
func TestUpdateRollsBackOnValidateFailure(t *testing.T) {
	m := newTestManager(t)
	m.loadFailed = false

	err := m.Update(func(cfg *AppConfig) error {
		cfg.Profiles = append(cfg.Profiles, ProcessProfile{ID: "p1", ProcessName: ""})
		return nil
	})
	if err == nil {
		t.Fatal("Update() обязан отклонять конфиг с пустым process_name")
	}
	if len(m.Get().Profiles) != 0 {
		t.Error("невалидный апдейт просочился в состояние менеджера")
	}
}

func TestUpdatePersistsToDisk(t *testing.T) {
	m := newTestManager(t)
	m.loadFailed = false

	if err := m.Update(func(cfg *AppConfig) error {
		cfg.Language = LanguageEN
		return nil
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	fresh := &Manager{configPath: m.configPath}
	if _, err := fresh.Load(); err != nil {
		t.Fatalf("Load() после Update error = %v", err)
	}
	if fresh.Get().Language != LanguageEN {
		t.Errorf("язык не записан на диск: %q", fresh.Get().Language)
	}
}

// Сценарий «пользователь испортил JSON руками»: Load обязан сохранить
// оригинал в .bak и запретить дальнейшие перезаписи — иначе первое же Update
// стёрло бы пользовательский файл дефолтами (баг B8).
func TestLoadBrokenJSONKeepsBackupAndBlocksSave(t *testing.T) {
	m := newTestManager(t)
	if err := os.WriteFile(m.configPath, []byte("{definitely not json"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Load(); err == nil {
		t.Fatal("Load() обязан падать на битом JSON")
	}

	if _, err := os.Stat(m.configPath + ".bak"); err != nil {
		t.Errorf("оригинал не сохранён в config.json.bak: %v", err)
	}

	err := m.Update(func(cfg *AppConfig) error {
		cfg.Language = LanguageEN
		return nil
	})
	if err == nil {
		t.Fatal("Update() обязан отказываться перезаписывать непрочитанный конфиг")
	}
}

func TestLoadCreatesDefaultWhenFileMissing(t *testing.T) {
	m := newTestManager(t)

	cfg, err := m.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Version == "" {
		t.Error("дефолтный конфиг без версии")
	}
	if _, err := os.Stat(m.configPath); err != nil {
		t.Errorf("файл конфига не создан: %v", err)
	}
	if !m.Loaded() {
		t.Error("Loaded() = false после успешного Load — сохранения останутся навсегда запрещены")
	}
}
