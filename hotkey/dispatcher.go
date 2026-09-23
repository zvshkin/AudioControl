package hotkey

import (
	"sync"

	"audio-control/audio"
	"audio-control/config"
	"audio-control/media"
)

type hotkeyEntry struct {
	profileID            string
	profileName          string
	processName          string
	targetAppUserModelId string
	stepPercent          int
	binding              config.HotkeyBinding
}

// MediaStatusFn вызывается, когда media-действие профиля не смогло найти целевую
// media-сессию процесса.
type MediaStatusFn func(profileID, processName string, action config.ActionType, available bool)

// ActionErrorFn вызывается при ошибке выполнения действия (например, у процесса
// нет активной аудиосессии). Раньше такие ошибки уходили в fmt.Println, которого
// в оконном Wails-приложении попросту не видно — пользователь получал "ничего не
// происходит" без единой подсказки, что именно не так.
type ActionErrorFn func(profileName string, action config.ActionType, message string)

// RegistrationStatus — результат попытки зарегистрировать одну комбинацию.
// Отдаётся во фронтенд, чтобы показать, какие хоткеи реально живы, а какие нет.
type RegistrationStatus struct {
	ProfileID   string            `json:"profile_id"`
	ProfileName string            `json:"profile_name"`
	Action      config.ActionType `json:"action"`
	KeyCombo    string            `json:"key_combo"`
	Registered  bool              `json:"registered"`
	Error       string            `json:"error,omitempty"`
}

type Dispatcher struct {
	// reloadMu сериализует ПОЛНОЕ перепостроение регистраций: Reload и
	// переключение глобального выключателя. Без него два одновременных
	// SaveProfile стартовали бы с одного nextID, перетирали бы друг другу
	// entries и гоняли Register/Unregister вразнобую — комбинации молча
	// «отваливались». Порядок захвата всегда: reloadMu -> mu; mu никогда не
	// удерживается при попытке взять reloadMu, поэтому взаимный клинч
	// невозможен (тот же приём, что в unregisterAll).
	reloadMu sync.Mutex

	mu       sync.Mutex
	cfgMgr   *config.Manager
	audioMgr *audio.AudioManager
	mediaMgr *media.Manager
	listener *Listener
	entries  map[int32]hotkeyEntry
	nextID   int32

	enabled       bool
	onMediaStatus MediaStatusFn
	onActionError ActionErrorFn
	lastStatus    []RegistrationStatus

	// jobs сериализует выполнение действий на отдельной горутине.
	// КРИТИЧНО: раньше действие выполнялось прямо в обработчике WM_HOTKEY, то
	// есть на потоке цикла сообщений. ChangeVolume поднимает COM, перечисляет
	// все аудиосессии и т.д. — на это уходят десятки-сотни миллисекунд, в
	// течение которых поток не читает очередь сообщений и не может обработать
	// ни следующий хоткей, ни собственные запросы на (пере)регистрацию. Плюс
	// инициализация STA COM на потоке, который сам обслуживает хоткеи, — лишний
	// риск реентрантности. Теперь поток сообщений только принимает WM_HOTKEY и
	// мгновенно передаёт работу сюда.
	jobs chan hotkeyEntry
	quit chan struct{}
	once sync.Once
}

func NewDispatcher(cfgMgr *config.Manager, audioMgr *audio.AudioManager, mediaMgr *media.Manager) *Dispatcher {
	d := &Dispatcher{
		cfgMgr:   cfgMgr,
		audioMgr: audioMgr,
		mediaMgr: mediaMgr,
		entries:  make(map[int32]hotkeyEntry),
		nextID:   1,
		enabled:  true,
		jobs:     make(chan hotkeyEntry, 32),
		quit:     make(chan struct{}),
	}

	d.listener = NewListener(d.handleHotkey)
	go d.worker()
	return d
}

func (d *Dispatcher) SetMediaStatusCallback(fn MediaStatusFn) {
	d.mu.Lock()
	d.onMediaStatus = fn
	d.mu.Unlock()
}

func (d *Dispatcher) SetActionErrorCallback(fn ActionErrorFn) {
	d.mu.Lock()
	d.onActionError = fn
	d.mu.Unlock()
}

// LastRegistrationStatus возвращает результат последней (пере)регистрации —
// используется фронтендом, чтобы показать реальное состояние каждого хоткея.
func (d *Dispatcher) LastRegistrationStatus() []RegistrationStatus {
	d.mu.Lock()
	out := make([]RegistrationStatus, len(d.lastStatus))
	copy(out, d.lastStatus)
	d.mu.Unlock()

	if d.cfgMgr == nil || len(out) == 0 {
		return out
	}

	names := make(map[string]string, len(out))
	for _, p := range d.cfgMgr.Get().Profiles {
		names[p.ID] = p.DisplayName
	}
	for i := range out {
		if name, ok := names[out[i].ProfileID]; ok {
			out[i].ProfileName = name
		}
	}
	return out
}

// currentProfileName возвращает акту DisplayName профиля по id. Нужно там, где
// имя — только текст для пользователя (ошибочные тосты): сама запись в
// entries может отставать от конфига, потому что переименование профиля
// намеренно не вызывает снятия и повторной регистрации комбинаций.
// Если профиль удалён — отдаёт сохранённое в записи имя (fallback).
func (d *Dispatcher) currentProfileName(id, fallback string) string {
	if d.cfgMgr == nil || id == "" {
		return fallback
	}
	for _, p := range d.cfgMgr.Get().Profiles {
		if p.ID == id {
			return p.DisplayName
		}
	}
	return fallback
}

func (d *Dispatcher) Start() error {
	if err := d.listener.Start(); err != nil {
		return err
	}
	return d.Reload()
}

func (d *Dispatcher) Stop() {
	d.unregisterAll()
	d.listener.Stop()
	d.once.Do(func() { close(d.quit) })
}

func (d *Dispatcher) SetGlobalEnabled(enabled bool) error {

	d.reloadMu.Lock()
	defer d.reloadMu.Unlock()

	d.mu.Lock()
	already := d.enabled == enabled
	d.enabled = enabled
	d.mu.Unlock()

	if already {
		return nil
	}

	if enabled {
		return d.reloadLocked()
	}

	d.unregisterAll()
	return nil
}

func (d *Dispatcher) IsGlobalEnabled() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.enabled
}

// unregisterAll снимает все текущие регистрации.
// ВАЖНО: список id снимается под мьютексом, а сами вызовы Unregister делаются
// уже БЕЗ него. Unregister блокируется до ответа потока слушателя, а тот, получив
// в этот момент WM_HOTKEY, вызвал бы handleHotkey, которому нужен тот же мьютекс —
// классический взаимный клинч, из-за которого приложение могло намертво зависнуть
// при выключении/перерегистрации хоткеев.
func (d *Dispatcher) unregisterAll() {
	d.mu.Lock()
	ids := make([]int32, 0, len(d.entries))
	for id := range d.entries {
		ids = append(ids, id)
	}
	d.entries = make(map[int32]hotkeyEntry)
	d.mu.Unlock()

	for _, id := range ids {
		d.listener.Unregister(id)
	}
}

// Reload перечитывает конфиг, снимает старые регистрации и ставит новые.
// Публичная точка входа: полное перепостроение идёт под reloadMu, чтобы два
// одновременных вызова (например, SaveProfile из UI и SetGlobalEnabled из
// трея) не гоняли Register/Unregister вразнобую и не перетирали entries.
func (d *Dispatcher) Reload() error {
	d.reloadMu.Lock()
	defer d.reloadMu.Unlock()
	return d.reloadLocked()
}

// reloadLocked — тело Reload. Вызывается строго с уже захваченным reloadMu
// (см. комментарий к полю reloadMu о порядке захвата reloadMu -> mu).
func (d *Dispatcher) reloadLocked() error {
	d.mu.Lock()
	enabled := d.enabled
	d.mu.Unlock()

	d.unregisterAll()

	if !enabled {
		d.mu.Lock()
		d.lastStatus = nil
		d.mu.Unlock()
		return nil
	}

	cfg := d.cfgMgr.Get()

	newEntries := make(map[int32]hotkeyEntry)
	statuses := make([]RegistrationStatus, 0)

	d.mu.Lock()
	id := d.nextID
	d.mu.Unlock()

	var firstErr error

	for _, profile := range cfg.Profiles {
		if !profile.Enabled {
			continue
		}

		step := profile.VolumeStepPercent
		if step <= 0 {
			step = config.DefaultVolumeStep
		}

		for _, hk := range profile.Hotkeys {
			if hk.KeyCode == 0 {
				continue
			}

			currentID := id
			id++

			status := RegistrationStatus{
				ProfileID:   profile.ID,
				ProfileName: profile.DisplayName,
				Action:      hk.Action,
				KeyCombo:    hk.KeyCombo,
			}

			if err := d.listener.Register(currentID, hk.Modifiers, hk.KeyCode); err != nil {

				status.Registered = false
				status.Error = err.Error()
				statuses = append(statuses, status)
				if firstErr == nil {
					firstErr = err
				}
				continue
			}

			status.Registered = true
			statuses = append(statuses, status)

			newEntries[currentID] = hotkeyEntry{
				profileID:            profile.ID,
				profileName:          profile.DisplayName,
				processName:          profile.ProcessName,
				targetAppUserModelId: profile.TargetAppUserModelId,
				stepPercent:          step,
				binding:              hk,
			}
		}
	}

	d.mu.Lock()
	d.entries = newEntries
	d.nextID = id
	d.lastStatus = statuses
	d.mu.Unlock()

	return firstErr
}

type Conflict struct {
	KeyCode   uint32            `json:"key_code"`
	Modifiers uint32            `json:"modifiers"`
	Bindings  []ConflictBinding `json:"bindings"`
}

type ConflictBinding struct {
	ProfileID   string            `json:"profile_id"`
	ProcessName string            `json:"process_name"`
	DisplayName string            `json:"display_name"`
	Action      config.ActionType `json:"action"`
	HotkeyID    string            `json:"hotkey_id"`
}

func FindConflicts(cfg config.AppConfig) []Conflict {
	type key struct {
		code uint32
		mods uint32
	}

	byCombo := make(map[key][]ConflictBinding)

	for _, profile := range cfg.Profiles {
		for _, hk := range profile.Hotkeys {
			if hk.KeyCode == 0 {
				continue
			}
			k := key{code: hk.KeyCode, mods: hk.Modifiers}
			byCombo[k] = append(byCombo[k], ConflictBinding{
				ProfileID:   profile.ID,
				ProcessName: profile.ProcessName,
				DisplayName: profile.DisplayName,
				Action:      hk.Action,
				HotkeyID:    hk.ID,
			})
		}
	}

	var conflicts []Conflict
	for k, bindings := range byCombo {
		if len(bindings) > 1 {
			conflicts = append(conflicts, Conflict{
				KeyCode:   k.code,
				Modifiers: k.mods,
				Bindings:  bindings,
			})
		}
	}

	return conflicts
}

// handleHotkey вызывается ИЗ ПОТОКА СООБЩЕНИЙ — здесь нельзя делать ничего
// долгого, только быстро снять запись и отдать её воркеру.
func (d *Dispatcher) handleHotkey(id int32) {
	d.mu.Lock()
	entry, ok := d.entries[id]
	enabled := d.enabled
	d.mu.Unlock()

	if !ok || !enabled {
		return
	}

	select {
	case d.jobs <- entry:
	default:

	}
}

func (d *Dispatcher) worker() {
	for {
		select {
		case entry := <-d.jobs:
			d.executeAction(entry)
		case <-d.quit:
			return
		}
	}
}

func (d *Dispatcher) executeAction(entry hotkeyEntry) {
	d.mu.Lock()
	statusFn := d.onMediaStatus
	errFn := d.onActionError
	d.mu.Unlock()

	entry.profileName = d.currentProfileName(entry.profileID, entry.profileName)

	hk := entry.binding

	step := entry.stepPercent
	if step <= 0 {
		step = config.DefaultVolumeStep
	}

	reportErr := func(err error) {
		if err != nil && errFn != nil {
			errFn(entry.profileName, hk.Action, err.Error())
		}
	}

	switch hk.Action {
	case config.ActionVolumeUp:
		_, err := d.audioMgr.ChangeVolume(0, entry.processName, step)
		reportErr(err)

	case config.ActionVolumeDown:
		_, err := d.audioMgr.ChangeVolume(0, entry.processName, -step)
		reportErr(err)

	case config.ActionToggleMute:
		_, err := d.audioMgr.ToggleMute(0, entry.processName)
		reportErr(err)

	case config.ActionPlayPause:
		d.controlMedia(entry, statusFn, errFn, media.ActionTogglePlayPause)

	case config.ActionNextTrack:
		d.controlMedia(entry, statusFn, errFn, media.ActionNext)

	case config.ActionPrevTrack:
		d.controlMedia(entry, statusFn, errFn, media.ActionPrevious)
	}
}

// controlMedia — единственное место, где media-хоткей превращается в действие.
// Цель ищется заново при КАЖДОМ нажатии (media.Manager.Control каждый раз зовёт
// GetSessions()), поэтому закрытие/перезапуск целевого приложения подхватывается
// сам собой, без отдельного наблюдателя за жизненным циклом сессий.
func (d *Dispatcher) controlMedia(entry hotkeyEntry, statusFn MediaStatusFn, errFn ActionErrorFn, action media.Action) {
	if d.mediaMgr == nil {
		return
	}

	ok, err := d.mediaMgr.Control(entry.processName, entry.targetAppUserModelId, action)
	if err != nil {
		if errFn != nil {
			errFn(entry.profileName, entry.binding.Action, err.Error())
		}
		return
	}

	if statusFn != nil {
		statusFn(entry.profileID, entry.processName, entry.binding.Action, ok)
	}
}
