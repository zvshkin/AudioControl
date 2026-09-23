// Package autostart управляет автозапуском приложения вместе с Windows через
// ключ реестра HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run.
//
// Почему именно HKCU, а не HKLM и не папка "Автозагрузка":
//   - HKCU не требует прав администратора (HKLM требует, и UAC-запрос при
//     переключении галочки в настройках выглядел бы крайне подозрительно);
//   - запись в реестре, в отличие от ярлыка в папке "Автозагрузка", не ломается
//     при переносе .exe и легко читается/снимается программно.
package autostart

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	valueName  = "AudioControlCentral"
)

// Enable прописывает текущий исполняемый файл в автозапуск.
func Enable() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("не удалось определить путь к исполняемому файлу: %w", err)
	}

	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("не удалось открыть ключ реестра автозапуска: %w", err)
	}
	defer key.Close()

	if err := key.SetStringValue(valueName, `"`+exePath+`"`); err != nil {
		return fmt.Errorf("не удалось записать значение автозапуска: %w", err)
	}

	return nil
}

// Disable убирает приложение из автозапуска. Отсутствие записи ошибкой не считается.
func Disable() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil
		}
		return fmt.Errorf("не удалось открыть ключ реестра автозапуска: %w", err)
	}
	defer key.Close()

	if err := key.DeleteValue(valueName); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("не удалось удалить значение автозапуска: %w", err)
	}

	return nil
}

// IsEnabled сообщает, включён ли автозапуск СЕЙЧАС по факту (читая реестр, а не
// сохранённое в конфиге намерение) — состояние могло измениться извне.
func IsEnabled() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	value, _, err := key.GetStringValue(valueName)
	if err != nil {
		return false
	}

	return strings.TrimSpace(value) != ""
}

// Sync приводит реестр в соответствие с желаемым состоянием. Дополнительно
// перезаписывает путь, если приложение переехало в другую папку — иначе запись
// в автозапуске указывала бы на несуществующий файл.
func Sync(desired bool) error {
	if !desired {
		return Disable()
	}

	exePath, err := os.Executable()
	if err != nil {
		return Enable()
	}

	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err == nil {
		current, _, readErr := key.GetStringValue(valueName)
		key.Close()

		if readErr == nil {
			expected := `"` + exePath + `"`
			if strings.EqualFold(strings.TrimSpace(current), expected) {
				return nil
			}
		}
	}

	return Enable()
}
