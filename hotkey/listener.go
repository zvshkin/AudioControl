package hotkey

import (
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Реальные флаги модификаторов Win32 RegisterHotKey (см. winuser.h).
const (
	ModAlt     uint32 = 0x0001
	ModControl uint32 = 0x0002
	ModShift   uint32 = 0x0004
	ModWin     uint32 = 0x0008

	// MOD_NOREPEAT (Windows 7+): без него удержание клавиши генерирует WM_HOTKEY
	// с частотой автоповтора клавиатуры — громкость улетала бы в 0 или 100 за
	// доли секунды. Добавляется автоматически ко всем регистрациям.
	modNoRepeat uint32 = 0x4000
)

var (
	// user32 нужен всему пакету hotkey; раньше он жил в media.go (см. A8 —
	// файл удалён вместе с мёртвым SendMediaKey) и висел бы на нём же.
	user32 = windows.NewLazySystemDLL("user32.dll")

	procRegisterHotKey     = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey   = user32.NewProc("UnregisterHotKey")
	procPostThreadMessageW = user32.NewProc("PostThreadMessageW")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procPeekMessageW       = user32.NewProc("PeekMessageW")
)

// listenerRequestTimeout — предел ожидания ответа потока слушателя на
// Register/Unregister (см. postRequest).
const listenerRequestTimeout = 5 * time.Second

const (
	wmHotkey = 0x0312
	wmQuit   = 0x0012
	wmApp    = 0x8000

	wmAppRegisterHotkey   = wmApp + 1
	wmAppUnregisterHotkey = wmApp + 2
)

type msgT struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

// hotkeyRequest — запрос на регистрацию/снятие, передаётся на поток слушателя.
type hotkeyRequest struct {
	unregister bool
	id         int32
	modifiers  uint32
	vk         uint32
	result     chan error
}

// CallbackFn вызывается при срабатывании хоткея по его числовому id.
type CallbackFn func(id int32)

// Listener владеет выделенным OS-потоком с циклом сообщений и регистрирует
// глобальные хоткеи через RegisterHotKey.
//
// ВАЖНО (это была причина, по которой хоткеи не работали вообще):
// раньше здесь создавалось скрытое message-only окно (HWND_MESSAGE), для него
// регистрировался класс и Go-колбэк wndProc через syscall.NewCallback, а
// WM_HOTKEY ожидался в wndProc после DispatchMessage. Это лишний и хрупкий слой:
// по документации RegisterHotKey, если hWnd = NULL, WM_HOTKEY кладётся прямо в
// очередь сообщений ПОТОКА, который зарегистрировал хоткей, и обрабатывается
// непосредственно в цикле GetMessage — без окна, без класса окна и без
// wndProc-колбэка. Именно так здесь теперь и сделано: минус ~100 строк
// низкоуровневого кода и минус все связанные с ним точки отказа.
//
// Следствие: RegisterHotKey/UnregisterHotKey ОБЯЗАНЫ вызываться с того же
// потока, что крутит цикл сообщений (хоткей привязывается к вызывающему потоку),
// поэтому запросы маршалятся туда через PostThreadMessage.
type Listener struct {
	mu       sync.Mutex
	threadID uint32
	running  bool
	callback CallbackFn
	ready    chan error
}

func NewListener(cb CallbackFn) *Listener {
	return &Listener{callback: cb}
}

// Start поднимает поток с циклом сообщений. Блокируется до готовности.
func (l *Listener) Start() error {
	l.mu.Lock()
	if l.running {
		l.mu.Unlock()
		return nil
	}
	l.ready = make(chan error, 1)
	l.mu.Unlock()

	go l.run()

	return <-l.ready
}

func (l *Listener) run() {

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	threadID := windows.GetCurrentThreadId()

	// Принудительно создаём очередь сообщений для этого потока до того, как
	// кто-либо сможет слать в неё PostThreadMessage: у только что созданного
	// потока очереди ещё нет, и ранние Post-сообщения были бы потеряны.
	// PeekMessage с PM_NOREMOVE — стандартный способ её материализовать.
	var tmp msgT
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&tmp)), 0, 0, 0, 0)

	l.mu.Lock()
	l.threadID = threadID
	l.running = true
	l.mu.Unlock()

	l.ready <- nil

	var msg msgT
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)

		if ret == 0 || uint32(ret) == 0xFFFFFFFF {
			break
		}

		switch msg.Message {
		case wmHotkey:

			if l.callback != nil {
				l.callback(int32(msg.WParam))
			}

		case wmAppRegisterHotkey, wmAppUnregisterHotkey:
			l.handleMarshaledRequest(&msg)
		}
	}

	l.mu.Lock()
	l.running = false
	l.threadID = 0
	l.mu.Unlock()
}

func (l *Listener) handleMarshaledRequest(msg *msgT) {
	req := (*hotkeyRequest)(unsafe.Pointer(msg.LParam))
	if req == nil {
		return
	}

	if req.unregister {
		procUnregisterHotKey.Call(0, uintptr(req.id))
		req.result <- nil
		return
	}

	ret, _, callErr := procRegisterHotKey.Call(
		0,
		uintptr(req.id),
		uintptr(req.modifiers|modNoRepeat),
		uintptr(req.vk),
	)
	if ret == 0 {
		req.result <- fmt.Errorf("RegisterHotKey failed: %v", callErr)
		return
	}
	req.result <- nil
}

// Register регистрирует глобальный хоткей. Безопасно вызывать из любой горутины —
// вызов маршалится на поток слушателя.
func (l *Listener) Register(id int32, modifiers uint32, vk uint32) error {
	return l.postRequest(&hotkeyRequest{
		id:        id,
		modifiers: modifiers,
		vk:        vk,
		result:    make(chan error, 1),
	}, wmAppRegisterHotkey)
}

// Unregister снимает регистрацию по id.
func (l *Listener) Unregister(id int32) {
	_ = l.postRequest(&hotkeyRequest{
		unregister: true,
		id:         id,
		result:     make(chan error, 1),
	}, wmAppUnregisterHotkey)
}

func (l *Listener) postRequest(req *hotkeyRequest, message uintptr) error {
	l.mu.Lock()
	threadID := l.threadID
	running := l.running
	l.mu.Unlock()

	if !running || threadID == 0 {
		return fmt.Errorf("listener is not running")
	}

	ok, _, postErr := procPostThreadMessageW.Call(
		uintptr(threadID),
		message,
		0,
		uintptr(unsafe.Pointer(req)),
	)
	if ok == 0 {
		return fmt.Errorf("PostThreadMessageW failed: %v", postErr)
	}

	select {
	case err := <-req.result:
		return err
	case <-time.After(listenerRequestTimeout):
		return fmt.Errorf("hotkey listener did not answer within %v", listenerRequestTimeout)
	}
}

// Stop завершает цикл сообщений слушателя.
func (l *Listener) Stop() {
	l.mu.Lock()
	threadID := l.threadID
	running := l.running
	l.mu.Unlock()

	if !running || threadID == 0 {
		return
	}

	procPostThreadMessageW.Call(uintptr(threadID), wmQuit, 0, 0)
}
