package process

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")

	procEnumWindows              = user32.NewProc("EnumWindows")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetWindowLongW           = user32.NewProc("GetWindowLongW")
	procExtractIconExW           = shell32.NewProc("ExtractIconExW")
	procDestroyIcon              = user32.NewProc("DestroyIcon")
	procGetIconInfo              = user32.NewProc("GetIconInfo")
	procGetObjectW               = gdi32.NewProc("GetObjectW")
	procGetDIBits                = gdi32.NewProc("GetDIBits")
	procDeleteObject             = gdi32.NewProc("DeleteObject")
	procGetDC                    = user32.NewProc("GetDC")
	procReleaseDC                = user32.NewProc("ReleaseDC")
)

const (
	GWL_STYLE      uint32 = 0xFFFFFFF0
	WS_VISIBLE            = 0x10000000
	BI_RGB                = 0
	DIB_RGB_COLORS        = 0
)

type BITMAP struct {
	Type         int32
	Width        int32
	Height       int32
	WidthBytes   int32
	Planes       uint16
	BitsPerPixel uint16
	Bits         uintptr
}

type BITMAPINFOHEADER struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type ICONINFO struct {
	IsIcon   int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  windows.Handle
	HbmColor windows.Handle
}

type ProcessScanner struct {
	mu sync.Mutex

	// visiblePIDs (PID -> заголовок окна) заполняется колбэком EnumWindows
	// (см. scanVisibleWindow) и живёт только на время одного GetActiveProcesses.
	visiblePIDs map[uint32]string

	// cb — колбэк для EnumWindows. Создаётся ОДИН РАЗ в NewScanner:
	// syscall.NewCallback занимает внутренний слот, который Go не освобождает
	// никогда, поэтому вариант «NewCallback на каждый вызов» (как было раньше)
	// рано или поздно упирается в лимит и ломает выбор процесса всему приложению.
	cb uintptr
}

func NewScanner() *ProcessScanner {
	s := &ProcessScanner{}
	s.cb = syscall.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
		return s.scanVisibleWindow(hwnd)
	})
	return s
}

// GetActiveProcesses возвращает отфильтрованный список процессов с окнами
func (ps *ProcessScanner) GetActiveProcesses() ([]ProcessInfo, error) {

	ps.mu.Lock()
	ps.visiblePIDs = make(map[uint32]string)
	procEnumWindows.Call(ps.cb, 0)
	visiblePIDs := ps.visiblePIDs
	ps.mu.Unlock()

	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to create process snapshot: %w", err)
	}
	defer windows.CloseHandle(snapshot)

	var procEntry windows.ProcessEntry32
	procEntry.Size = uint32(unsafe.Sizeof(procEntry))

	err = windows.Process32First(snapshot, &procEntry)
	var results []ProcessInfo

	for err == nil {
		pid := procEntry.ProcessID
		windowTitle, hasWindow := visiblePIDs[pid]

		if hasWindow {
			exeName := windows.UTF16ToString(procEntry.ExeFile[:])

			if !isSystemProcess(exeName) {
				fullPath := getProcessPath(pid)
				iconBase64 := extractProcessIconBase64(fullPath)

				displayName := windowTitle
				if displayName == "" {
					displayName = strings.TrimSuffix(exeName, filepath.Ext(exeName))
				}

				results = append(results, ProcessInfo{
					PID:         pid,
					ProcessName: strings.ToLower(exeName),
					DisplayName: displayName,
					IconBase64:  iconBase64,
					HasWindow:   true,
				})
			}
		}

		err = windows.Process32Next(snapshot, &procEntry)
	}

	return results, nil
}

// scanVisibleWindow обрабатывает одно окно из EnumWindows. Всегда возвращает 1
// («продолжить перечисление»): отбор окон идёт не здесь — метод лишь наполняет
// ps.visiblePIDs, а фильтрация выполняется по уже собранной таблице.
// Вызывается только с потока, держащего ps.mu (см. GetActiveProcesses).
func (ps *ProcessScanner) scanVisibleWindow(hwnd uintptr) uintptr {
	visible, _, _ := procIsWindowVisible.Call(hwnd)
	if visible == 0 {
		return 1
	}

	style, _, _ := procGetWindowLongW.Call(hwnd, uintptr(GWL_STYLE))
	if style&WS_VISIBLE == 0 {
		return 1
	}

	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return 1
	}

	// Получаем заголовок окна
	var title [256]uint16
	procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
	windowTitle := windows.UTF16ToString(title[:])

	if strings.TrimSpace(windowTitle) != "" && windowTitle != "Program Manager" {
		if _, exists := ps.visiblePIDs[pid]; !exists {
			ps.visiblePIDs[pid] = windowTitle
		}
	}

	return 1
}

// Извлечение пути к .exe файлу по PID
func getProcessPath(pid uint32) string {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(handle)

	var buf [windows.MAX_PATH]uint16
	size := uint32(len(buf))
	err = windows.QueryFullProcessImageName(handle, 0, &buf[0], &size)
	if err != nil {
		return ""
	}

	return windows.UTF16ToString(buf[:size])
}

// Извлечение иконки приложения и конвертация в PNG Base64
func extractProcessIconBase64(exePath string) string {
	if exePath == "" {
		return ""
	}

	pathPtr, err := windows.UTF16PtrFromString(exePath)
	if err != nil {
		return ""
	}

	var hIcon windows.Handle

	ret, _, _ := procExtractIconExW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		uintptr(unsafe.Pointer(&hIcon)),
		0,
		1,
	)

	if ret == 0 || hIcon == 0 {
		return ""
	}
	defer procDestroyIcon.Call(uintptr(hIcon))

	img, err := iconToImage(hIcon)
	if err != nil {
		return ""
	}

	var buff bytes.Buffer
	if err := png.Encode(&buff, img); err != nil {
		return ""
	}

	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buff.Bytes())
}

// Преобразование Win32 HICON в image.RGBA
func iconToImage(hIcon windows.Handle) (image.Image, error) {
	var iconInfo ICONINFO
	ret, _, _ := procGetIconInfo.Call(uintptr(hIcon), uintptr(unsafe.Pointer(&iconInfo)))
	if ret == 0 {
		return nil, fmt.Errorf("failed GetIconInfo")
	}
	if iconInfo.HbmColor != 0 {
		defer procDeleteObject.Call(uintptr(iconInfo.HbmColor))
	}
	if iconInfo.HbmMask != 0 {
		defer procDeleteObject.Call(uintptr(iconInfo.HbmMask))
	}

	if iconInfo.HbmColor == 0 {
		return nil, fmt.Errorf("icon has no color bitmap")
	}

	var bmp BITMAP
	got, _, _ := procGetObjectW.Call(uintptr(iconInfo.HbmColor), unsafe.Sizeof(bmp), uintptr(unsafe.Pointer(&bmp)))
	if got == 0 {
		return nil, fmt.Errorf("failed to read icon bitmap header")
	}

	width := int(bmp.Width)
	if width < 0 {
		width = -width
	}
	height := int(bmp.Height)
	if height < 0 {
		height = -height
	}

	if width == 0 || height == 0 || width > 1024 || height > 1024 {
		return nil, fmt.Errorf("invalid icon bitmap size: %dx%d", width, height)
	}

	hdc, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, hdc)

	var bmi BITMAPINFOHEADER
	bmi.Size = uint32(unsafe.Sizeof(bmi))
	bmi.Width = int32(width)
	bmi.Height = -int32(height)
	bmi.Planes = 1
	bmi.BitCount = 32
	bmi.Compression = BI_RGB

	pixels := make([]byte, width*height*4)
	procGetDIBits.Call(
		hdc,
		uintptr(iconInfo.HbmColor),
		0,
		uintptr(height),
		uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(unsafe.Pointer(&bmi)),
		DIB_RGB_COLORS,
	)

	img := image.NewRGBA(image.Rect(0, 0, width, height))

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := (y*width + x) * 4
			b := pixels[idx]
			g := pixels[idx+1]
			r := pixels[idx+2]
			a := pixels[idx+3]

			offset := (y*width + x) * 4
			img.Pix[offset] = r
			img.Pix[offset+1] = g
			img.Pix[offset+2] = b
			img.Pix[offset+3] = a
		}
	}

	return img, nil
}

func isSystemProcess(name string) bool {
	sys := []string{"explorer.exe", "dwm.exe", "cmd.exe", "powershell.exe", "conhost.exe", "svchost.exe", "taskmgr.exe"}
	lower := strings.ToLower(name)
	for _, s := range sys {
		if lower == s {
			return true
		}
	}
	return false
}
