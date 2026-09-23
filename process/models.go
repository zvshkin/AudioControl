package process

// ProcessInfo описывает активный процесс Windows для UI.
//
// A8: поле HasAudio удалено — оно всегда было false (WASAPI никогда его не
// проставлял, вопреки старому комментарию), а факт наличия аудиосессии UI
// получает отдельно и надёжнее — через GetProfileStates (ProfileState.HasSession).
type ProcessInfo struct {
	PID         uint32 `json:"pid"`          // Process ID
	ProcessName string `json:"process_name"` // Имя файла ("spotify.exe")
	DisplayName string `json:"display_name"` // Читаемое имя ("Spotify Free")
	IconBase64  string `json:"icon_base64"`  // Иконка в формате "data:image/png;base64,..."
	HasWindow   bool   `json:"has_window"`   // Есть ли видимое окно
}
