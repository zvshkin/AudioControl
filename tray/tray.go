// Package tray реализует минимальную иконку в системном трее Windows поверх
// голых syscalls (Shell_NotifyIcon), без сторонних зависимостей — в том же стиле,
// что и остальной проект (audio/, process/, hotkey/).
//
// Раньше в проекте не было НИКАКОЙ реализации трея, при этом кнопка "свернуть в
// трей" в тайтлбаре вызывала WindowHide() — окно пропадало насовсем, вернуть его
// было нечем. Этот пакет добавляет реальную иконку с меню "Показать" / "Выход".
package tray

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procLoadIconW           = user32.NewProc("LoadIconW")
	procLoadImageW          = user32.NewProc("LoadImageW")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenuW         = user32.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procShellNotifyIconW    = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")
)

const (
	hwndMessage = ^uintptr(2) // (HWND)-3 — окно-только-для-сообщений, без UI

	wmDestroy    = 0x0002
	wmClose      = 0x0010
	wmCommand    = 0x0111
	wmLButtonUp  = 0x0202
	wmLButtonDbl = 0x0203
	wmRButtonUp  = 0x0205
	wmTrayIcon   = 0x8000 + 1 // WM_APP + 1 — сообщение обратного вызова от Shell_NotifyIcon

	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	idiApplication = 32512 // MAKEINTRESOURCE(IDI_APPLICATION)

	imageIcon      = 1
	lrLoadFromFile = 0x00000010
	lrDefaultSize  = 0x00000040

	mfString       = 0x00000000
	mfChecked      = 0x00000008
	mfSeparator    = 0x00000800
	mfGrayed       = 0x00000001
	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100

	cmdShow          = 1001
	cmdExit          = 1002
	cmdToggleHotkeys = 1003
)

// Labels — подписи пунктов меню. Вынесены наружу, чтобы приложение могло
// передать их на нужном языке (см. настройку языка интерфейса) и обновить
// на лету при его смене.
type Labels struct {
	Show           string
	HotkeysEnabled string
	Exit           string
}

func defaultLabels() Labels {
	return Labels{
		Show:           "Показать",
		HotkeysEnabled: "Горячие клавиши",
		Exit:           "Выход",
	}
}

type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     windows.Handle
	HIcon         windows.Handle
	HCursor       windows.Handle
	HbrBackground windows.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       windows.Handle
}

type msgT struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

type point struct{ X, Y int32 }

// notifyIconData зеркалит NOTIFYICONDATAW (используем только нужные нам поля,
// но раскладка структуры должна точно совпадать с WinAPI, включая размер cbSize).
type notifyIconData struct {
	CbSize           uint32
	Hwnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            windows.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UTimeoutOrVer    uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     windows.Handle
}

// Options настраивает поведение трея.
type Options struct {
	// Tooltip — подсказка при наведении на иконку.
	Tooltip string
	// IconBytes — содержимое .ico-файла (например, встроенное через go:embed).
	// Предпочтительнее IconPath: собранный .exe не тащит рядом с собой исходный
	// .ico (он нужен только на этапе сборки, чтобы прописать иконку самого
	// .exe), поэтому путь к файлу на диске в готовом приложении не сработает.
	IconBytes []byte
	// IconPath — путь к .ico файлу. Если пусто — используется системная иконка по умолчанию.
	IconPath string
	// OnShow вызывается по левому клику/двойному клику или пункту меню "Показать".
	OnShow func()
	// OnExit вызывается по пункту меню "Выход".
	OnExit func()
	// OnToggleHotkeys вызывается по пункту меню "Хоткеи включены" — должен реально
	// переключить регистрацию хоткеев (а не просто визуальное состояние) и вернуть
	// новое фактическое состояние (после переключения).
	OnToggleHotkeys func() bool
	// HotkeysEnabled — начальное состояние галочки в меню при старте.
	HotkeysEnabled bool
	// Labels — подписи пунктов меню (для локализации). Пустые поля заменяются
	// русскими значениями по умолчанию.
	Labels Labels
}

// Tray — запущенный экземпляр иконки в трее.
type Tray struct {
	opts     Options
	mu       sync.Mutex
	hwnd     uintptr
	ready    chan error
	stopOnce sync.Once

	// done закрывается потоком трея при выходе из цикла сообщений — Stop
	// ждёт его, чтобы к моменту возврата окно гарантированно было уничтожено
	// на СВОЁМ потоке (см. комментарий в Stop).
	done chan struct{}

	hotkeysEnabled bool
	labels         Labels
}

func New(opts Options) *Tray {
	labels := opts.Labels
	def := defaultLabels()
	if labels.Show == "" {
		labels.Show = def.Show
	}
	if labels.HotkeysEnabled == "" {
		labels.HotkeysEnabled = def.HotkeysEnabled
	}
	if labels.Exit == "" {
		labels.Exit = def.Exit
	}

	return &Tray{opts: opts, hotkeysEnabled: opts.HotkeysEnabled, labels: labels}
}

// SetLabels меняет подписи меню (например, при смене языка интерфейса).
// Меню строится заново при каждом открытии, поэтому изменения применяются сразу.
func (t *Tray) SetLabels(labels Labels) {
	t.mu.Lock()
	if labels.Show != "" {
		t.labels.Show = labels.Show
	}
	if labels.HotkeysEnabled != "" {
		t.labels.HotkeysEnabled = labels.HotkeysEnabled
	}
	if labels.Exit != "" {
		t.labels.Exit = labels.Exit
	}
	t.mu.Unlock()
}

// SetHotkeysEnabled синхронизирует галочку в меню с фактическим состоянием —
// нужно, когда хоткеи переключили не через трей, а из окна настроек.
func (t *Tray) SetHotkeysEnabled(enabled bool) {
	t.mu.Lock()
	t.hotkeysEnabled = enabled
	t.mu.Unlock()
}

// Start поднимает скрытое окно-приёмник сообщений трея и добавляет иконку.
// Блокируется до готовности (или ошибки).
func (t *Tray) Start() error {
	t.mu.Lock()
	if t.hwnd != 0 {
		t.mu.Unlock()
		return nil
	}
	t.ready = make(chan error, 1)
	t.done = make(chan struct{})
	t.mu.Unlock()

	go t.run()

	return <-t.ready
}

func (t *Tray) run() {

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	defer func() {
		t.mu.Lock()
		if t.done != nil {
			close(t.done)
			t.done = nil
		}
		t.mu.Unlock()
	}()

	className, err := windows.UTF16PtrFromString("VolumeHotkeysTrayWindow")
	if err != nil {
		t.ready <- fmt.Errorf("UTF16PtrFromString failed: %w", err)
		return
	}

	hInstance, _, _ := procGetModuleHandleW.Call(0)
	wndProcPtr := syscall.NewCallback(t.wndProc)

	wc := wndClassExW{
		LpfnWndProc:   wndProcPtr,
		HInstance:     windows.Handle(hInstance),
		LpszClassName: className,
	}
	wc.CbSize = uint32(unsafe.Sizeof(wc))

	atom, _, regErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		t.ready <- fmt.Errorf("RegisterClassExW failed: %v", regErr)
		return
	}

	hwnd, _, createErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		0,
		0, 0, 0, 0, 0,
		hwndMessage,
		0,
		hInstance,
		0,
	)
	if hwnd == 0 {
		t.ready <- fmt.Errorf("CreateWindowExW failed: %v", createErr)
		return
	}

	t.mu.Lock()
	t.hwnd = hwnd
	t.mu.Unlock()

	if err := t.addIcon(hwnd, hInstance); err != nil {
		t.ready <- err
		return
	}

	t.ready <- nil

	var msg msgT
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if ret == 0 || ret == ^uintptr(0) {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	t.removeIcon(hwnd)
}

func (t *Tray) loadIcon(hInstance uintptr) windows.Handle {
	if len(t.opts.IconBytes) > 0 {
		if h := t.loadIconFromBytes(t.opts.IconBytes); h != 0 {
			return h
		}

	}

	if t.opts.IconPath != "" {
		pathPtr, err := windows.UTF16PtrFromString(t.opts.IconPath)
		if err == nil {
			h, _, _ := procLoadImageW.Call(
				0,
				uintptr(unsafe.Pointer(pathPtr)),
				imageIcon,
				0, 0,
				lrLoadFromFile|lrDefaultSize,
			)
			if h != 0 {
				return windows.Handle(h)
			}
		}

	}

	h, _, _ := procLoadIconW.Call(0, uintptr(idiApplication))
	return windows.Handle(h)
}

// loadIconFromBytes создаёт HICON из сырых байт .ico-файла. LoadImageW умеет
// грузить иконки только с диска (LR_LOADFROMFILE), а не из памяти напрямую,
// поэтому байты временно пишутся во временный файл. Сам HICON создаётся
// синхронно внутри вызова LoadImageW, поэтому файл можно удалить сразу же —
// системе он после возврата из функции уже не нужен.
func (t *Tray) loadIconFromBytes(data []byte) windows.Handle {
	tmpFile, err := os.CreateTemp("", "audiocontrol-tray-*.ico")
	if err != nil {
		return 0
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	_, writeErr := tmpFile.Write(data)
	tmpFile.Close()
	if writeErr != nil {
		return 0
	}

	pathPtr, err := windows.UTF16PtrFromString(tmpPath)
	if err != nil {
		return 0
	}

	h, _, _ := procLoadImageW.Call(
		0,
		uintptr(unsafe.Pointer(pathPtr)),
		imageIcon,
		0, 0,
		lrLoadFromFile|lrDefaultSize,
	)

	return windows.Handle(h)
}

func (t *Tray) addIcon(hwnd uintptr, hInstance uintptr) error {
	var nid notifyIconData
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.Hwnd = hwnd
	nid.UID = 1
	nid.UFlags = nifMessage | nifIcon | nifTip
	nid.UCallbackMessage = wmTrayIcon
	nid.HIcon = t.loadIcon(hInstance)

	tip := t.opts.Tooltip
	if tip == "" {
		tip = "VolumeHotkeys"
	}
	copyStringToUTF16(nid.SzTip[:], tip)

	ret, _, callErr := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	if ret == 0 {
		return fmt.Errorf("Shell_NotifyIconW(NIM_ADD) failed: %v", callErr)
	}
	return nil
}

func (t *Tray) removeIcon(hwnd uintptr) {
	var nid notifyIconData
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.Hwnd = hwnd
	nid.UID = 1
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
}

func (t *Tray) wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmTrayIcon:
		switch lParam {
		case wmLButtonUp, wmLButtonDbl:
			if t.opts.OnShow != nil {
				t.opts.OnShow()
			}
		case wmRButtonUp:
			t.showContextMenu(hwnd)
		}
		return 0

	case wmCommand:
		t.handleCommand(loWord(wParam))
		return 0

	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0

	default:
		ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
		return ret
	}
}

// handleCommand выполняет действие по id пункта меню. Вынесен отдельно, потому
// что TrackPopupMenu вызывается с флагом TPM_RETURNCMD — а этот флаг ЯВНО
// отключает автоматическую отправку WM_COMMAND (так задокументировано в Win32:
// "The function does not send a message notifying the window of the user's
// selection"). Раньше код полагался именно на это сообщение в wndProc, поэтому
// ни один пункт меню — включая "Выход" — по факту никогда не срабатывал.
// Теперь результат TrackPopupMenu обрабатывается напрямую в showContextMenu;
// case wmCommand выше оставлен на случай, если сообщение всё же придёт из
// другого источника.
func (t *Tray) handleCommand(cmd uint32) {
	switch cmd {
	case cmdShow:
		if t.opts.OnShow != nil {
			t.opts.OnShow()
		}
	case cmdExit:
		if t.opts.OnExit != nil {
			t.opts.OnExit()
		}
	case cmdToggleHotkeys:
		if t.opts.OnToggleHotkeys != nil {
			newState := t.opts.OnToggleHotkeys()
			t.mu.Lock()
			t.hotkeysEnabled = newState
			t.mu.Unlock()
		}
	}
}

func (t *Tray) showContextMenu(hwnd uintptr) {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	t.mu.Lock()
	enabled := t.hotkeysEnabled
	labels := t.labels
	t.mu.Unlock()

	showText, _ := windows.UTF16PtrFromString(labels.Show)
	toggleText, _ := windows.UTF16PtrFromString(labels.HotkeysEnabled)
	exitText, _ := windows.UTF16PtrFromString(labels.Exit)

	toggleFlags := uintptr(mfString)
	if enabled {
		toggleFlags |= mfChecked
	}

	procAppendMenuW.Call(hMenu, mfString, uintptr(cmdShow), uintptr(unsafe.Pointer(showText)))
	procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
	procAppendMenuW.Call(hMenu, toggleFlags, uintptr(cmdToggleHotkeys), uintptr(unsafe.Pointer(toggleText)))
	procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
	procAppendMenuW.Call(hMenu, mfString, uintptr(cmdExit), uintptr(unsafe.Pointer(exitText)))

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	procSetForegroundWindow.Call(hwnd)

	ret, _, _ := procTrackPopupMenu.Call(
		hMenu,
		tpmRightButton|tpmReturnCmd,
		uintptr(pt.X),
		uintptr(pt.Y),
		0,
		hwnd,
		0,
	)

	if ret != 0 {
		t.handleCommand(uint32(ret))
	}
}

// Stop убирает иконку и завершает поток трея.
//
// B10 (гонка кросс-поточного DestroyWindow): раньше Stop дёргал DestroyWindow
// напрямую с ВЫЗЫВАЮЩЕГО потока, а окно принадлежит потоку трея — Win32 это
// формально допускает, но оконная процедура при этом выполняется на чужом
// потоке, что грозит рассинхроном с wndProc-колбэком и, в худшем случае,
// зависанием при завершении. Безопасный путь — доставить WM_CLOSE в поток
// владельца окна: там же DefWindowProc вызовет DestroyWindow, wndProc получит
// WM_DESTROY и постит WM_QUIT, цикл GetMessage выходит сам и снимает иконку.
// Done-канал ждём с таймаутом: если поток уже мёртв (или завис) — не вечно.
func (t *Tray) Stop() {
	t.stopOnce.Do(func() {
		t.mu.Lock()
		hwnd := t.hwnd
		done := t.done
		t.hwnd = 0
		t.mu.Unlock()

		if hwnd != 0 {
			procPostMessageW.Call(hwnd, wmClose, 0, 0)
		}

		if done != nil {
			select {
			case <-done:
			case <-time.After(2 * time.Second):

				fmt.Fprintln(os.Stderr, "tray: message thread did not stop within 2s")
			}
		}
	})
}

func loWord(v uintptr) uint32 {
	return uint32(v) & 0xFFFF
}

func copyStringToUTF16(dst []uint16, s string) {
	src, err := windows.UTF16FromString(s)
	if err != nil {

		return
	}
	n := len(src)
	if n > len(dst) {
		n = len(dst)
	}
	copy(dst[:n], src[:n])
}
