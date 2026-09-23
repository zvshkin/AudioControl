// Package media реализует управление воспроизведением конкретного приложения через
// GlobalSystemMediaTransportControlsSessionManager (WinRT, Windows.Media.Control).
//
// ЗАЧЕМ ЭТОТ ПАКЕТ СУЩЕСТВУЕТ:
// Раньше media-хоткеи (Play/Pause/Next/Previous) отправлялись как глобальные
// синтетические клавиши через SendInput (функция SendMediaKey удалена в ходе
// чистки мёртвого кода — A8).
// Windows маршрутизирует такие клавиши на GetCurrentSession() — сессию, которую
// система считает "текущей" по своей внутренней эвристике. У этого подхода нет
// понятия "чей это хоткей" — если два приложения играют музыку одновременно,
// хоткей профиля Z может внезапно управлять приложением V. Этот пакет вместо
// глобальной клавиши находит КОНКРЕТНУЮ media-сессию нужного процесса через
// GetSessions() и вызывает Try*Async именно на ней.
//
// РЕАЛИЗАЦИЯ И ЕЁ ГРАНИЦЫ:
// GlobalSystemMediaTransportControlsSessionManager — чистый WinRT-класс (не COM
// Automation/IDispatch), поэтому обычные библиотеки вроде go-ole тут не подходят —
// пришлось обращаться к нему через сырые WinRT-вызовы (RoGetActivationFactory +
// вызовы по смещениям в vtable). Смещения ниже подтверждены ДВУМЯ независимыми
// источниками: официальные Rust-биндинги Microsoft (windows-rs, которые генерируются
// напрямую из метаданных Windows и сохраняют настоящий порядок объявления методов —
// в отличие от документации MS Learn, где методы всегда отсортированы по алфавиту)
// и рабочий пример на AutoHotkey с тем же GUID активации. Тем не менее это одна из
// самых низкоуровневых частей проекта — необходимо реальное тестирование на Windows.
package media

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var combase = windows.NewLazySystemDLL("combase.dll")

var (
	procRoInitialize              = combase.NewProc("RoInitialize")
	procRoUninitialize            = combase.NewProc("RoUninitialize")
	procRoGetActivationFactory    = combase.NewProc("RoGetActivationFactory")
	procWindowsCreateString       = combase.NewProc("WindowsCreateString")
	procWindowsDeleteString       = combase.NewProc("WindowsDeleteString")
	procWindowsGetStringRawBuffer = combase.NewProc("WindowsGetStringRawBuffer")
)

const (
	roInitMultithreaded = 1 // RO_INIT_MULTITHREADED — сессии GSMTC помечены как "Agile" (можно звать из любого потока MTA)

	sOK                = 0x00000000
	sFalse             = 0x00000001
	eIllegalMethodCall = 0x8000000E // WinRT возвращает это, пока IAsyncOperation ещё не завершилась
)

const sessionManagerClassName = "Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager"

// IID интерфейса IGlobalSystemMediaTransportControlsSessionManagerStatics.
// {2050C4EE-11A0-57DE-AED7-C97C70338245}
var iidSessionManagerStatics = windows.GUID{
	Data1: 0x2050c4ee,
	Data2: 0x11a0,
	Data3: 0x57de,
	Data4: [8]byte{0xae, 0xd7, 0xc9, 0x7c, 0x70, 0x33, 0x82, 0x45},
}

// Смещения слотов в vtable (считая от начала: 0-2 = IUnknown, 3-5 = IInspectable,
// далее — методы конкретного интерфейса в порядке их объявления).
const (
	// IGlobalSystemMediaTransportControlsSessionManagerStatics
	slotRequestAsync = 6

	// GlobalSystemMediaTransportControlsSessionManager (инстанс)
	slotGetCurrentSession = 6
	slotGetSessions       = 7

	// Windows.Foundation.Collections.IVectorView<T>
	slotVectorGetAt = 6
	slotVectorSize  = 7

	// GlobalSystemMediaTransportControlsSession
	slotSourceAppUserModelId = 6
	slotTryPlayAsync         = 10
	slotTryPauseAsync        = 11
	slotTrySkipNextAsync     = 16
	slotTrySkipPreviousAsync = 17
	slotTryTogglePlayPause   = 20

	// Windows.Foundation.IAsyncOperation<T>
	slotAsyncGetResults = 8
)

// Action — действие, которое нужно выполнить над найденной media-сессией.
type Action int

const (
	ActionPlay Action = iota
	ActionPause
	ActionTogglePlayPause
	ActionNext
	ActionPrevious
)

func vtableCall(obj uintptr, slot int, args ...uintptr) uint32 {
	if obj == 0 {
		return 0x80004003
	}

	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fnPtr := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(slot)*unsafe.Sizeof(uintptr(0))))

	callArgs := make([]uintptr, 0, len(args)+1)
	callArgs = append(callArgs, obj)
	callArgs = append(callArgs, args...)

	r1, _, _ := syscall.SyscallN(fnPtr, callArgs...)
	return uint32(r1)
}

func hrFailed(hr uint32) bool {
	return hr&0x80000000 != 0
}

func hrError(hr uint32) error {
	return fmt.Errorf("HRESULT 0x%08X", hr)
}

func releaseObj(obj uintptr) {
	if obj == 0 {
		return
	}
	vtableCall(obj, 2)
}

func createHString(s string) (uintptr, error) {
	utf16, err := windows.UTF16FromString(s)
	if err != nil {
		return 0, err
	}
	length := len(utf16) - 1

	var hstr uintptr
	hr, _, _ := procWindowsCreateString.Call(
		uintptr(unsafe.Pointer(&utf16[0])),
		uintptr(length),
		uintptr(unsafe.Pointer(&hstr)),
	)
	if uint32(hr) != sOK {
		return 0, fmt.Errorf("WindowsCreateString failed: %s", hrError(uint32(hr)))
	}
	return hstr, nil
}

func deleteHString(hstr uintptr) {
	if hstr == 0 {
		return
	}
	procWindowsDeleteString.Call(hstr)
}

func hstringToString(hstr uintptr) string {
	if hstr == 0 {
		return ""
	}
	var length uint32
	ptr, _, _ := procWindowsGetStringRawBuffer.Call(hstr, uintptr(unsafe.Pointer(&length)))
	if ptr == 0 || length == 0 {
		return ""
	}
	slice := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), length)
	return windows.UTF16ToString(slice)
}

// waitAsyncOperationPtr опрашивает IAsyncOperation<T> (T — WinRT-объект/интерфейс)
// через GetResults, пока та не завершится. WinRT возвращает E_ILLEGAL_METHOD_CALL,
// если результат ещё не готов — это и используется как сигнал "подождать ещё".
// Так надёжнее и на порядок проще, чем реализовывать полноценный
// IAsyncOperationCompletedHandler (отдельный COM-объект со своей vtable) только
// ради коллбэка о завершении.
func waitAsyncOperationPtr(asyncOp uintptr, timeout time.Duration) (uintptr, error) {
	deadline := time.Now().Add(timeout)
	for {
		var resultPtr uintptr
		hr := vtableCall(asyncOp, slotAsyncGetResults, uintptr(unsafe.Pointer(&resultPtr)))
		if hr == sOK {
			return resultPtr, nil
		}
		if hr != eIllegalMethodCall {
			return 0, hrError(hr)
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("timeout waiting for WinRT async operation")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// waitAsyncOperationBool — то же самое, но для IAsyncOperation<bool> (результат
// Try*Async методов сессии). Результат — однобайтовый WinRT boolean, поэтому нужен
// отдельный вариант с byte-адресом вместо uintptr-адреса для out-параметра.
func waitAsyncOperationBool(asyncOp uintptr, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		var resultByte byte
		hr := vtableCall(asyncOp, slotAsyncGetResults, uintptr(unsafe.Pointer(&resultByte)))
		if hr == sOK {
			return resultByte != 0, nil
		}
		if hr != eIllegalMethodCall {
			return false, hrError(hr)
		}
		if time.Now().After(deadline) {
			return false, fmt.Errorf("timeout waiting for WinRT async operation")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func requestSessionManager(timeout time.Duration) (uintptr, error) {
	classNameHStr, err := createHString(sessionManagerClassName)
	if err != nil {
		return 0, err
	}
	defer deleteHString(classNameHStr)

	var factory uintptr
	hr, _, _ := procRoGetActivationFactory.Call(
		classNameHStr,
		uintptr(unsafe.Pointer(&iidSessionManagerStatics)),
		uintptr(unsafe.Pointer(&factory)),
	)
	if uint32(hr) != sOK || factory == 0 {
		return 0, fmt.Errorf("RoGetActivationFactory failed: %s", hrError(uint32(hr)))
	}
	defer releaseObj(factory)

	var asyncOp uintptr
	hr2 := vtableCall(factory, slotRequestAsync, uintptr(unsafe.Pointer(&asyncOp)))
	if hrFailed(hr2) || asyncOp == 0 {
		return 0, fmt.Errorf("RequestAsync call failed: %s", hrError(hr2))
	}
	defer releaseObj(asyncOp)

	manager, err := waitAsyncOperationPtr(asyncOp, timeout)
	if err != nil {
		return 0, fmt.Errorf("RequestAsync did not complete: %w", err)
	}
	return manager, nil
}

// getSessions возвращает список сессий. Каждый элемент — это НОВАЯ ссылка,
// владение переходит вызывающему коду (нужно вызвать releaseObj на каждой, которая
// не будет использована дальше).
func getSessions(manager uintptr) ([]uintptr, error) {
	var vectorView uintptr
	hr := vtableCall(manager, slotGetSessions, uintptr(unsafe.Pointer(&vectorView)))
	if hrFailed(hr) || vectorView == 0 {
		return nil, hrError(hr)
	}
	defer releaseObj(vectorView)

	var size uint32
	hr = vtableCall(vectorView, slotVectorSize, uintptr(unsafe.Pointer(&size)))
	if hrFailed(hr) {
		return nil, hrError(hr)
	}

	sessions := make([]uintptr, 0, size)
	for i := uint32(0); i < size; i++ {
		var sessionPtr uintptr
		hr := vtableCall(vectorView, slotVectorGetAt, uintptr(i), uintptr(unsafe.Pointer(&sessionPtr)))
		if hrFailed(hr) || sessionPtr == 0 {
			continue
		}
		sessions = append(sessions, sessionPtr)
	}
	return sessions, nil
}

func getSourceAppUserModelId(session uintptr) string {
	var hstr uintptr
	hr := vtableCall(session, slotSourceAppUserModelId, uintptr(unsafe.Pointer(&hstr)))
	if hrFailed(hr) {
		return ""
	}
	defer deleteHString(hstr)
	return hstringToString(hstr)
}

func trySessionAction(session uintptr, slot int, timeout time.Duration) (bool, error) {
	var asyncOp uintptr
	hr := vtableCall(session, slot, uintptr(unsafe.Pointer(&asyncOp)))
	if hrFailed(hr) || asyncOp == 0 {
		return false, fmt.Errorf("session action call failed: %s", hrError(hr))
	}
	defer releaseObj(asyncOp)

	return waitAsyncOperationBool(asyncOp, timeout)
}

type job struct {
	fn   func()
	done chan struct{}
}

type Manager struct {
	jobs chan job
	quit chan struct{}
	once sync.Once
}

func NewManager() *Manager {
	m := &Manager{
		jobs: make(chan job),
		quit: make(chan struct{}),
	}
	go m.comThread()
	return m
}

func (m *Manager) comThread() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hr, _, _ := procRoInitialize.Call(uintptr(roInitMultithreaded))
	if uint32(hr) == sOK || uint32(hr) == sFalse {
		defer procRoUninitialize.Call()
	}

	for {
		select {
		case j := <-m.jobs:
			j.fn()
			close(j.done)
		case <-m.quit:
			return
		}
	}
}

// do выполняет fn синхронно на потоке, владеющем WinRT-апартаментом, и блокируется
// до завершения. Использовать closures для получения результатов (см. Control).
func (m *Manager) do(fn func()) {
	done := make(chan struct{})
	select {
	case m.jobs <- job{fn: fn, done: done}:
		<-done
	case <-m.quit:
	}
}

// Stop останавливает фоновый поток WinRT. Вызывать при закрытии приложения.
func (m *Manager) Stop() {
	m.once.Do(func() { close(m.quit) })
}

// findSessionLocked ищет среди активных media-сессий подходящую под processName
// (или под targetAppUserModelId, если он задан — для случая, когда у процесса
// несколько сессий и профиль явно закреплён за одной из них). ДОЛЖНА вызываться
// только изнутри do().
//
// Возвращает (0, "", nil), если подходящая сессия сейчас не найдена — это не
// ошибка, а статус "цель недоступна прямо сейчас", который вызывающий код должен
// показать пользователю, а не тихо проигнорировать и тем более не заменить
// обращением к глобальной "текущей" сессии.
func findSessionLocked(processName, targetAppUserModelId string) (uintptr, string, error) {
	manager, err := requestSessionManager(3 * time.Second)
	if err != nil {
		return 0, "", fmt.Errorf("не удалось получить SessionManager: %w", err)
	}
	defer releaseObj(manager)

	sessions, err := getSessions(manager)
	if err != nil {
		return 0, "", fmt.Errorf("не удалось получить список media-сессий: %w", err)
	}

	targetLower := strings.ToLower(strings.TrimSpace(targetAppUserModelId))

	var foundSession uintptr
	var foundAumid string

	for _, s := range sessions {
		aumid := getSourceAppUserModelId(s)

		matches := false
		if targetLower != "" {

			matches = strings.ToLower(strings.TrimSpace(aumid)) == targetLower
		} else {
			matches = aumidMatchesProcess(aumid, processName)
		}

		if matches && foundSession == 0 {
			foundSession = s
			foundAumid = aumid
			continue
		}

		releaseObj(s)
	}

	return foundSession, foundAumid, nil
}

// aumidMatchesProcess пытается сопоставить media-сессию с именем процесса.
//
// ВАЖНО: SourceAppUserModelId — это НЕ имя исполняемого файла. У обычного Win32-
// приложения там часто действительно лежит "app.exe", но у приложения с
// зарегистрированным AppUserModelID (в том числе у Яндекс Музыки, Spotify из
// Store, браузеров) это может быть совершенно другая строка. Поэтому раньше
// сравнение strings.Contains(aumid, "яндекс музыка.exe") не срабатывало никогда,
// хотя сессия существовала и была видна в Win+A.
//
// Здесь мы пробуем несколько разумных вариантов сопоставления, а если ни один не
// сработал — вызывающий код честно сообщает "цель не найдена", и пользователь
// может закрепить нужную сессию вручную через ListTargets (который специально
// возвращает ВЕСЬ список, а не отфильтрованный этой же эвристикой).
func aumidMatchesProcess(aumid, processName string) bool {
	a := normalizeForMatch(aumid)
	p := normalizeForMatch(processName)

	if a == "" || p == "" {
		return false
	}

	pBase := strings.TrimSuffix(p, ".exe")

	aTail := a
	for _, sep := range []string{"!", "\\", "/"} {
		if idx := strings.LastIndex(aTail, sep); idx >= 0 && idx+1 < len(aTail) {
			aTail = aTail[idx+1:]
		}
	}
	aTailBase := strings.TrimSuffix(aTail, ".exe")

	candidates := []string{p, pBase}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if a == c || aTail == c || aTailBase == c {
			return true
		}
		if strings.Contains(a, c) || strings.Contains(aTail, c) {
			return true
		}

		if len(aTailBase) >= 4 && strings.Contains(c, aTailBase) {
			return true
		}
	}

	return false
}

// normalizeForMatch приводит строку к нижнему регистру и убирает пробелы —
// "Яндекс Музыка.exe" и "яндексмузыка.exe" должны считаться одним и тем же.
func normalizeForMatch(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), " ", "")
}

// Control находит media-сессию процесса processName (либо конкретную сессию по
// targetAppUserModelId, если задан) и выполняет над ней action.
//
// ok=false, err=nil означает "подходящая сессия сейчас не найдена" — процесс не
// публикует media-сессию прямо сейчас (закрыт, ещё не начал играть и т.п.).
// Вызывающий код (hotkey.Dispatcher) должен на это как-то отреагировать
// (например, показать toast), а не считать действие выполненным.
func (m *Manager) Control(processName, targetAppUserModelId string, action Action) (ok bool, err error) {
	m.do(func() {
		session, _, findErr := findSessionLocked(processName, targetAppUserModelId)
		if findErr != nil {
			err = findErr
			return
		}
		if session == 0 {
			return
		}
		defer releaseObj(session)

		var slot int
		switch action {
		case ActionPlay:
			slot = slotTryPlayAsync
		case ActionPause:
			slot = slotTryPauseAsync
		case ActionNext:
			slot = slotTrySkipNextAsync
		case ActionPrevious:
			slot = slotTrySkipPreviousAsync
		default:
			slot = slotTryTogglePlayPause
		}

		success, ctrlErr := trySessionAction(session, slot, 3*time.Second)
		if ctrlErr != nil {
			err = ctrlErr
			return
		}
		ok = success
	})
	return ok, err
}

// ListTargetsBatch возвращает AppUserModelId всех текущих media-сессий,
// сопоставленных КАЖДОМУ процессу из processNames, за ОДИН WinRT-обход.
//
// Зачем батч: раньше фронт звал ListTargets по разу на каждый медиа-профиль,
// и каждый вызов заново поднимал SessionManager (RoGetActivationFactory +
// RequestAsync + GetSessions) — N полных WinRT-обходов на каждое открытие
// экрана вместо одного.
//
// Смысл «процесс -> все сессии» сохранён из ListTargets НАМЕРЕННО: список
// существует для ручного выбора цели, когда эвристика aumidMatchesProcess не
// сработала, поэтому фильтровать его этой же эвристикой нельзя — в пикере
// снова было бы пусто ровно в тот момент, когда он нужен больше всего.
func (m *Manager) ListTargetsBatch(processNames []string) (map[string][]string, error) {
	var out map[string][]string
	var err error

	m.do(func() {
		manager, reqErr := requestSessionManager(3 * time.Second)
		if reqErr != nil {
			err = reqErr
			return
		}
		defer releaseObj(manager)

		sessions, sessErr := getSessions(manager)
		if sessErr != nil {
			err = sessErr
			return
		}

		// Собираем весь список сессий один раз. Каждому процессу отдаём
		// СВОЮ копию слайса: общий backing array между записями карты —
		// почва для будущей мутационной ошибки.
		var all []string
		for _, s := range sessions {
			aumid := getSourceAppUserModelId(s)
			if strings.TrimSpace(aumid) != "" {
				all = append(all, aumid)
			}
			releaseObj(s)
		}

		out = make(map[string][]string, len(processNames))
		for _, name := range processNames {
			if name == "" {
				continue
			}
			if _, exists := out[name]; exists {
				continue
			}
			cp := make([]string, len(all))
			copy(cp, all)
			out[name] = cp
		}
	})

	return out, err
}

// ListTargets — одиночная обёртка над ListTargetsBatch для одного процесса.
func (m *Manager) ListTargets(processName string) (targets []string, err error) {
	byProcess, err := m.ListTargetsBatch([]string{processName})
	if err != nil {
		return nil, err
	}
	return byProcess[processName], nil
}
