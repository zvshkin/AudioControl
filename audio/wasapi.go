package audio

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
)

// GUID и IID константы для Windows Audio Session API
var (
	CLSID_MMDeviceEnumerator = ole.NewGUID("{BCDE0395-E52F-467C-8E3D-C4579291692E}")
	IID_IMMDeviceEnumerator  = ole.NewGUID("{A95664D2-9614-4F35-A746-DE8DB63617E6}")
	// ВНИМАНИЕ: оба GUID ниже раньше были указаны НЕВЕРНО, и это была причина
	// ошибки 0x80004002 (E_NOINTERFACE) при вызове IMMDevice::Activate — то есть
	// громкость не работала вообще. Значения сверены с заголовком Windows SDK
	// audiopolicy.h / audioclient.h. Менять их нельзя.
	IID_IAudioSessionManager2 = ole.NewGUID("{77AA99A0-1BD6-484F-8BC7-2C654C9A9B6F}")
	IID_IAudioSessionControl2 = ole.NewGUID("{BFB7FF88-7239-4FC9-8FA2-07C950BE9C6D}")
	IID_ISimpleAudioVolume    = ole.NewGUID("{87CE5498-68D6-44E5-9215-6DA47EF883D8}")
)

// Константы WASAPI
const (
	eRender              = 0
	eMultimedia          = 1
	CLSCTX_INPROC_SERVER = 0x1
	S_FALSE              = 0x00000001
)

var (
	// ole32 нужен ровно для одного — освобождать LPWSTR из
	// IAudioSessionControl2::GetSessionIdentifier: COM выделяет такую строку
	// через CoTaskMemAlloc, и владение после вызова переходит нам.
	ole32 = syscall.NewLazyDLL("ole32.dll")

	// procCoTaskMemFree. Без вызова строка течёт на КАЖДОЙ аудиосессии при
	// КАЖДОМ обращении к audio-менеджеру, а поллинг профилей зовёт его каждые
	// несколько секунд — на живущем сутками трее это была бы непрерывная утечка.
	procCoTaskMemFree = ole32.NewProc("CoTaskMemFree")
)

// VTable структуры для прямого вызова COM методов
type IMMDeviceEnumeratorVtbl struct {
	ole.IUnknownVtbl
	EnumAudioEndpoints                     uintptr
	GetDefaultAudioEndpoint                uintptr
	GetDevice                              uintptr
	RegisterEndpointNotificationCallback   uintptr
	UnregisterEndpointNotificationCallback uintptr
}

type IMMDeviceEnumerator struct {
	LpVtbl *IMMDeviceEnumeratorVtbl
}

type IMMDeviceVtbl struct {
	ole.IUnknownVtbl
	Activate          uintptr
	OpenPropertyStore uintptr
	GetId             uintptr
	GetState          uintptr
}

type IMMDevice struct {
	LpVtbl *IMMDeviceVtbl
}

type IAudioSessionManager2Vtbl struct {
	ole.IUnknownVtbl
	GetAudioSessionControl          uintptr
	GetSimpleAudioVolume            uintptr
	GetSessionEnumerator            uintptr
	RegisterSessionNotification     uintptr
	UnregisterSessionNotification   uintptr
	RegisterSessionNotificationEx   uintptr
	UnregisterSessionNotificationEx uintptr
}

type IAudioSessionManager2 struct {
	LpVtbl *IAudioSessionManager2Vtbl
}

type IAudioSessionEnumeratorVtbl struct {
	ole.IUnknownVtbl
	GetCount   uintptr
	GetSession uintptr
}

type IAudioSessionEnumerator struct {
	LpVtbl *IAudioSessionEnumeratorVtbl
}

type IAudioSessionControl2Vtbl struct {
	ole.IUnknownVtbl
	// IAudioSessionControl
	GetState                           uintptr
	GetDisplayName                     uintptr
	SetDisplayName                     uintptr
	GetIconPath                        uintptr
	SetIconPath                        uintptr
	GetGroupingParam                   uintptr
	SetGroupingParam                   uintptr
	RegisterAudioSessionNotification   uintptr
	UnregisterAudioSessionNotification uintptr
	// IAudioSessionControl2
	GetSessionIdentifier         uintptr
	GetSessionInstanceIdentifier uintptr
	GetProcessId                 uintptr
	IsSystemSoundsSession        uintptr
	SetDuckingPreference         uintptr
}

type IAudioSessionControl2 struct {
	LpVtbl *IAudioSessionControl2Vtbl
}

type ISimpleAudioVolumeVtbl struct {
	ole.IUnknownVtbl
	SetMasterVolume uintptr
	GetMasterVolume uintptr
	SetMute         uintptr
	GetMute         uintptr
}

type ISimpleAudioVolume struct {
	LpVtbl *ISimpleAudioVolumeVtbl
}

// AudioManager предоставляет методы управления звуком процессов
type AudioManager struct{}

func NewAudioManager() *AudioManager {
	return &AudioManager{}
}

// walkSessions перебирает ВСЕ активные аудиосессии за ОДИН COM-обход
// (CoInitialize -> MMDeviceEnumerator -> default endpoint ->
// IAudioSessionManager2 -> IAudioSessionEnumerator).
//
// Зачем каркас выделен: раньше каждое обращение к громкости само поднимало всё
// это окружение, а GetProfileStates зовёт аудио-API ОДИН РАЗ НА ПРОФИЛЬ —
// поллинг UI (каждые 3 с) при N профилях стоил N полных обходов, каждый со
// своим CoInitialize и Activate. Теперь все цели обслуживаются одним обходом
// (см. GetVolumesBatch).
//
// visit получает PID сессии, её идентификатор в НИЖНЕМ РЕГИСТРЕ (копия:
// оригинал через CoTaskMemFree освобождается до вызова — ср. историю утечки
// B3) и ISimpleAudioVolume либо nil, если QueryInterface не удался. Возврат
// stop=true прерывает перебор, ошибка — оборвывает весь обход и возвращается
// вызывающему. Владение vol освобождается здесь же, сразу после visit.
func (am *AudioManager) walkSessions(visit func(pid uint32, idLower string, vol *ISimpleAudioVolume) (stop bool, err error)) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		if oleErr, ok := err.(*ole.OleError); ok && uint32(oleErr.Code()) != S_FALSE {
			return fmt.Errorf("CoInitializeEx failed: %w", err)
		}
	}
	defer ole.CoUninitialize()

	unknown, err := ole.CreateInstance(CLSID_MMDeviceEnumerator, IID_IMMDeviceEnumerator)
	if err != nil {
		return fmt.Errorf("failed to create MMDeviceEnumerator: %w", err)
	}
	enumerator := (*IMMDeviceEnumerator)(unsafe.Pointer(unknown))
	defer releaseComObject((*ole.IUnknown)(unsafe.Pointer(enumerator)))

	// 2. Получаем стандартное устройство воспроизведения
	var device *IMMDevice
	hr, _, _ := syscall.SyscallN(
		enumerator.LpVtbl.GetDefaultAudioEndpoint,
		uintptr(unsafe.Pointer(enumerator)),
		uintptr(eRender),
		uintptr(eMultimedia),
		uintptr(unsafe.Pointer(&device)),
	)
	if hr != 0 {
		return fmt.Errorf("GetDefaultAudioEndpoint failed with hr: 0x%X", hr)
	}
	defer releaseComObject((*ole.IUnknown)(unsafe.Pointer(device)))

	// 3. Активируем IAudioSessionManager2
	var manager *IAudioSessionManager2
	hr, _, _ = syscall.SyscallN(
		device.LpVtbl.Activate,
		uintptr(unsafe.Pointer(device)),
		uintptr(unsafe.Pointer(IID_IAudioSessionManager2)),
		uintptr(CLSCTX_INPROC_SERVER),
		0,
		uintptr(unsafe.Pointer(&manager)),
	)
	if hr != 0 {
		return fmt.Errorf("failed to activate IAudioSessionManager2 with hr: 0x%X", hr)
	}
	defer releaseComObject((*ole.IUnknown)(unsafe.Pointer(manager)))

	// 4. Перечисляем аудионаборы
	var sessionEnum *IAudioSessionEnumerator
	hr, _, _ = syscall.SyscallN(
		manager.LpVtbl.GetSessionEnumerator,
		uintptr(unsafe.Pointer(manager)),
		uintptr(unsafe.Pointer(&sessionEnum)),
	)
	if hr != 0 {
		return fmt.Errorf("GetSessionEnumerator failed with hr: 0x%X", hr)
	}
	defer releaseComObject((*ole.IUnknown)(unsafe.Pointer(sessionEnum)))

	var count int32
	syscall.SyscallN(
		sessionEnum.LpVtbl.GetCount,
		uintptr(unsafe.Pointer(sessionEnum)),
		uintptr(unsafe.Pointer(&count)),
	)

	for i := 0; i < int(count); i++ {
		var sessionControl *ole.IUnknown
		hr, _, _ = syscall.SyscallN(
			sessionEnum.LpVtbl.GetSession,
			uintptr(unsafe.Pointer(sessionEnum)),
			uintptr(i),
			uintptr(unsafe.Pointer(&sessionControl)),
		)
		if hr != 0 || sessionControl == nil {
			continue
		}

		sessionControl2Disp, err := sessionControl.QueryInterface(IID_IAudioSessionControl2)
		releaseComObject(sessionControl)

		if err != nil || sessionControl2Disp == nil {
			continue
		}
		sessionControl2 := (*IAudioSessionControl2)(unsafe.Pointer(sessionControl2Disp))

		var sessionPID uint32
		syscall.SyscallN(
			sessionControl2.LpVtbl.GetProcessId,
			uintptr(unsafe.Pointer(sessionControl2)),
			uintptr(unsafe.Pointer(&sessionPID)),
		)

		// GetSessionIdentifier отдаёт LPWSTR, выделенный через CoTaskMemAlloc, —
		// владение строкой остаётся у нас, и без Free она текла бы на КАЖДОЙ
		// сессии при КАЖДОМ вызове (поллинг профилей дёргает audio-менеджер
		// каждые несколько секунд — утечка копилась бы часами работы в трее).
		// При ошибке указателю не доверяем (out-параметр не определён) —
		// ни читать, ни освобождать его нельзя.
		var sessionIdentifier *uint16
		hrIdent, _, _ := syscall.SyscallN(
			sessionControl2.LpVtbl.GetSessionIdentifier,
			uintptr(unsafe.Pointer(sessionControl2)),
			uintptr(unsafe.Pointer(&sessionIdentifier)),
		)
		idLower := ""
		if hrIdent == 0 && sessionIdentifier != nil {
			idLower = strings.ToLower(ole.UTF16PtrToString(sessionIdentifier))
			procCoTaskMemFree.Call(uintptr(unsafe.Pointer(sessionIdentifier)))
		}

		// ISimpleAudioVolume снимаем здесь, чтобы у visit был готовый объект:
		// отдельный матчинг под каждый запрос громкости внутри цикла был бы
		// лишним COM-вызовом на каждую сессию.
		var simpleVolume *ISimpleAudioVolume
		hrVol, _, _ := syscall.SyscallN(
			sessionControl2.LpVtbl.QueryInterface,
			uintptr(unsafe.Pointer(sessionControl2)),
			uintptr(unsafe.Pointer(IID_ISimpleAudioVolume)),
			uintptr(unsafe.Pointer(&simpleVolume)),
		)
		releaseComObject((*ole.IUnknown)(unsafe.Pointer(sessionControl2)))
		if hrVol != 0 {
			simpleVolume = nil
		}

		stop, visitErr := visit(sessionPID, idLower, simpleVolume)

		if simpleVolume != nil {
			releaseComObject((*ole.IUnknown)(unsafe.Pointer(simpleVolume)))
		}
		if visitErr != nil {
			return visitErr
		}
		if stop {
			return nil
		}
	}

	return nil
}

// withAudioVolume находит активную аудио-сессию процесса по PID или ExeName и
// выполняет callback — одиночная обёртка над walkSessions, сохранившая
// прежнюю семантику: работает с ПЕРВОЙ подошедшей сессией, отсутствие сессии —
// ошибка.
func (am *AudioManager) withAudioVolume(pid uint32, exeName string, callback func(volume *ISimpleAudioVolume) error) error {
	exeLower := strings.ToLower(exeName)
	found := false

	err := am.walkSessions(func(sessionPID uint32, idLower string, vol *ISimpleAudioVolume) (bool, error) {
		matches := false
		if pid > 0 && sessionPID == pid {
			matches = true
		} else if exeLower != "" && strings.Contains(idLower, exeLower) {
			matches = true
		}

		if !matches || vol == nil {

			return false, nil
		}

		found = true
		return true, callback(vol)
	})
	if err != nil {
		return err
	}

	if !found {
		return fmt.Errorf("active audio session not found for PID: %d / Exe: %s", pid, exeName)
	}
	return nil
}

// SessionVolume — результат батч-чтения громкости одного процесса.
type SessionVolume struct {
	Volume int  `json:"volume"`
	Muted  bool `json:"muted"`
	Found  bool `json:"found"` // есть активная аудиосессия прямо сейчас
}

// GetVolumesBatch читает громкость/mute сразу для всех exeNames за ОДИН
// COM-обход (см. walkSessions). Ключ результата — исходное имя процесса;
// процесс без активной сессии попадает в карту с Found=false — это статус
// «сейчас не звучит», а не ошибка (ровно так GetProfileStates трактовал
// ошибку GetVolume и раньше). Матчинг сессии тот же, что в withAudioVolume:
// идентификатор сессии содержит имя exe'шника.
func (am *AudioManager) GetVolumesBatch(exeNames []string) map[string]SessionVolume {
	results := make(map[string]SessionVolume, len(exeNames))

	pending := make(map[string]string, len(exeNames))
	for _, name := range exeNames {
		if name == "" {
			continue
		}
		if _, ok := results[name]; !ok {
			results[name] = SessionVolume{}
		}
		lower := strings.ToLower(name)
		if _, ok := pending[lower]; !ok {
			pending[lower] = name
		}
	}
	if len(pending) == 0 {
		return results
	}

	_ = am.walkSessions(func(_ uint32, idLower string, vol *ISimpleAudioVolume) (bool, error) {
		if vol == nil {
			return false, nil
		}

		for lower, name := range pending {
			if !strings.Contains(idLower, lower) {
				continue
			}

			var v float32
			hr, _, _ := syscall.SyscallN(
				vol.LpVtbl.GetMasterVolume,
				uintptr(unsafe.Pointer(vol)),
				uintptr(unsafe.Pointer(&v)),
			)
			if hr != 0 {
				continue
			}

			var muted uint32
			hr, _, _ = syscall.SyscallN(
				vol.LpVtbl.GetMute,
				uintptr(unsafe.Pointer(vol)),
				uintptr(unsafe.Pointer(&muted)),
			)
			if hr != 0 {
				continue
			}

			results[name] = SessionVolume{
				Volume: int(math.Round(float64(v * 100))),
				Muted:  muted != 0,
				Found:  true,
			}
			delete(pending, lower)
		}

		return len(pending) == 0, nil
	})

	return results
}

// GetVolume возвращает текущий уровень громкости процесса (0..100) и статус Mute
func (am *AudioManager) GetVolume(pid uint32, exeName string) (int, bool, error) {
	var currentVolume float32
	var isMuted bool

	err := am.withAudioVolume(pid, exeName, func(vol *ISimpleAudioVolume) error {
		hr, _, _ := syscall.SyscallN(
			vol.LpVtbl.GetMasterVolume,
			uintptr(unsafe.Pointer(vol)),
			uintptr(unsafe.Pointer(&currentVolume)),
		)
		if hr != 0 {
			return fmt.Errorf("GetMasterVolume failed: 0x%X", hr)
		}

		var muted uint32
		hr, _, _ = syscall.SyscallN(
			vol.LpVtbl.GetMute,
			uintptr(unsafe.Pointer(vol)),
			uintptr(unsafe.Pointer(&muted)),
		)
		if hr != 0 {
			return fmt.Errorf("GetMute failed: 0x%X", hr)
		}

		isMuted = muted != 0
		return nil
	})

	if err != nil {
		return 0, false, err
	}

	return int(math.Round(float64(currentVolume * 100))), isMuted, nil
}

// ChangeVolume изменяет громкость процесса на deltaPercent (+10 или -10)
func (am *AudioManager) ChangeVolume(pid uint32, exeName string, deltaPercent int) (int, error) {
	var newVolPercent int

	err := am.withAudioVolume(pid, exeName, func(vol *ISimpleAudioVolume) error {
		var currentVol float32
		syscall.SyscallN(
			vol.LpVtbl.GetMasterVolume,
			uintptr(unsafe.Pointer(vol)),
			uintptr(unsafe.Pointer(&currentVol)),
		)

		targetVol := currentVol + (float32(deltaPercent) / 100.0)
		if targetVol > 1.0 {
			targetVol = 1.0
		}
		if targetVol < 0.0 {
			targetVol = 0.0
		}

		hr, _, _ := syscall.SyscallN(
			vol.LpVtbl.SetMasterVolume,
			uintptr(unsafe.Pointer(vol)),
			uintptr(math.Float32bits(targetVol)),
			0,
		)
		if hr != 0 {
			return fmt.Errorf("SetMasterVolume failed with hr: 0x%X", hr)
		}

		newVolPercent = int(math.Round(float64(targetVol * 100)))
		return nil
	})

	return newVolPercent, err
}

// ToggleMute инвертирует режим без звука (Mute / Unmute)
func (am *AudioManager) ToggleMute(pid uint32, exeName string) (bool, error) {
	var newMuteState bool

	err := am.withAudioVolume(pid, exeName, func(vol *ISimpleAudioVolume) error {
		var muted uint32
		syscall.SyscallN(
			vol.LpVtbl.GetMute,
			uintptr(unsafe.Pointer(vol)),
			uintptr(unsafe.Pointer(&muted)),
		)

		newMute := uint32(0)
		if muted == 0 {
			newMute = 1
		}

		hr, _, _ := syscall.SyscallN(
			vol.LpVtbl.SetMute,
			uintptr(unsafe.Pointer(vol)),
			uintptr(newMute),
			0,
		)
		if hr != 0 {
			return fmt.Errorf("SetMute failed with hr: 0x%X", hr)
		}

		newMuteState = newMute == 1
		return nil
	})

	return newMuteState, err
}

// SetVolume выставляет АБСОЛЮТНУЮ громкость процесса (0..100), в отличие от
// ChangeVolume, который меняет её относительно текущей. Нужно для слайдера в UI.
func (am *AudioManager) SetVolume(pid uint32, exeName string, percent int) (int, error) {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	target := float32(percent) / 100.0

	err := am.withAudioVolume(pid, exeName, func(vol *ISimpleAudioVolume) error {
		hr, _, _ := syscall.SyscallN(
			vol.LpVtbl.SetMasterVolume,
			uintptr(unsafe.Pointer(vol)),
			uintptr(math.Float32bits(target)),
			0,
		)
		if hr != 0 {
			return fmt.Errorf("SetMasterVolume failed with hr: 0x%X", hr)
		}
		return nil
	})

	return percent, err
}

// SetMute выставляет КОНКРЕТНОЕ состояние mute, а не инвертирует текущее
// (ToggleMute). Нужно, чтобы кнопка в интерфейсе не «разъезжалась» с реальным
// состоянием при параллельных изменениях из микшера Windows.
func (am *AudioManager) SetMute(pid uint32, exeName string, muted bool) error {
	value := uint32(0)
	if muted {
		value = 1
	}

	return am.withAudioVolume(pid, exeName, func(vol *ISimpleAudioVolume) error {
		hr, _, _ := syscall.SyscallN(
			vol.LpVtbl.SetMute,
			uintptr(unsafe.Pointer(vol)),
			uintptr(value),
			0,
		)
		if hr != 0 {
			return fmt.Errorf("SetMute failed with hr: 0x%X", hr)
		}
		return nil
	})
}

func releaseComObject(obj *ole.IUnknown) {
	if obj != nil {
		obj.Release()
	}
}
