package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"unsafe"

	"audio-control/config"

	"golang.org/x/sys/windows"
)

const maxLogSizeBytes = 512 * 1024

// appLog живёт до initLogger: даже ошибка самой инициализации лога не должна
// приводить к nil-панике.
var appLog = log.New(io.Discard, "", log.LstdFlags)

var (
	mbUser32        = windows.NewLazySystemDLL("user32.dll")
	procMessageBoxW = mbUser32.NewProc("MessageBoxW")
)

const (
	mbOk        = 0x00000000
	mbIconError = 0x00000010
)

// initLogger поднимает лог-файл %AppData%\AudioControl\app.log.
//
// Лог дописывается в конец; при перерастании maxLogSizeBytes прошлая копия
// отодвигается в app.log.old — утилита диагностики, а не аудит, копиться в
// ечно не должна. Если файл недоступен — катимся на stderr, чтобы
// диагностика не исчезала совсем.
func initLogger() {
	appLog.SetFlags(log.LstdFlags)
	appLog.SetPrefix("AudioControl ")

	dir, err := os.UserConfigDir()
	if err == nil {
		dir = filepath.Join(dir, config.ConfigDirName)
		if mkErr := os.MkdirAll(dir, 0755); mkErr == nil {
			path := filepath.Join(dir, "app.log")

			if fi, stErr := os.Stat(path); stErr == nil && fi.Size() > maxLogSizeBytes {
				_ = os.Rename(path, path+".old")
			}

			if f, openErr := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); openErr == nil {
				appLog.SetOutput(io.MultiWriter(os.Stderr, f))
				return
			}
		}
	}

	appLog.SetOutput(os.Stderr)
}

// showStartupError показывает модальное системное окно с ошибкой. Нужно там,
// где UI ещё не готов или сама подсистема мертва (например, трей) — в лог такое
// сообщение при живой программе пользователь всё равно не откроет.
func showStartupError(title, text string) {
	u16title, err1 := windows.UTF16PtrFromString(title)
	u16text, err2 := windows.UTF16PtrFromString(text)
	if err1 != nil || err2 != nil {
		return
	}

	procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(u16text)),
		uintptr(unsafe.Pointer(u16title)),
		mbOk|mbIconError,
	)
}
