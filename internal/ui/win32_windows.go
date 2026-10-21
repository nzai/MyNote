//go:build windows

package ui

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
	"github.com/rodrigocfd/windigo/wstr"
)

const (
	documentWindowClass       = "ClipNote.Document"
	ControllerWindowClassName = "ClipNote.Controller"
	controllerWindowClass     = ControllerWindowClassName
)

var (
	processInstance win.HINSTANCE
	instanceOnce    sync.Once
	instanceErr     error

	windowProcOnce sync.Once
	windowProc     uintptr

	classMu    sync.Mutex
	classAtoms = make(map[string]win.ATOM)
)

type nativeWindow interface {
	handleNativeMessage(co.WM, win.WPARAM, win.LPARAM) (uintptr, bool)
}

type nativeWindowState struct {
	hwnd    win.HWND
	handler nativeWindow
}

func currentInstance() (win.HINSTANCE, error) {
	instanceOnce.Do(func() {
		processInstance, instanceErr = win.GetModuleHandle("")
		if instanceErr != nil {
			instanceErr = fmt.Errorf("get process module handle: %w", instanceErr)
		}
	})
	return processInstance, instanceErr
}

func registerNativeClass(name string) (win.ATOM, error) {
	classMu.Lock()
	defer classMu.Unlock()
	if atom, ok := classAtoms[name]; ok {
		return atom, nil
	}

	instance, err := currentInstance()
	if err != nil {
		return 0, err
	}
	cursor, err := win.HINSTANCE(0).LoadCursor(win.CursorResIdc(co.IDC_ARROW))
	if err != nil {
		return 0, fmt.Errorf("load arrow cursor: %w", err)
	}

	var encodedName wstr.BufEncoder
	class := win.WNDCLASSEX{
		Style:         co.CS_DBLCLKS,
		LpfnWndProc:   nativeWindowProc(),
		HInstance:     instance,
		HCursor:       cursor,
		HbrBackground: win.HBRUSH(co.COLOR_WINDOW + 1),
		LpszClassName: (*uint16)(encodedName.AllowEmpty(name)),
	}
	class.SetCbSize()

	atom, err := win.RegisterClassEx(&class)
	if err == nil {
		classAtoms[name] = atom
		return atom, nil
	}
	if errors.Is(err, co.ERROR_CLASS_ALREADY_EXISTS) {
		return 0, fmt.Errorf("window class %q is already registered outside this process", name)
	}
	return 0, fmt.Errorf("register Win32 window class %q: %w", name, err)
}

func nativeWindowProc() uintptr {
	windowProcOnce.Do(func() {
		windowProc = syscall.NewCallback(
			func(hwnd win.HWND, msg co.WM, wParam win.WPARAM, lParam win.LPARAM) uintptr {
				state := nativeWindowStateFor(hwnd, msg, lParam)
				if state == nil {
					return hwnd.DefWindowProc(msg, wParam, lParam)
				}

				ret, handled := state.handler.handleNativeMessage(msg, wParam, lParam)
				if msg == co.WM_NCDESTROY {
					_, _ = hwnd.SetWindowLongPtr(co.GWLP_USERDATA, 0)
					state.hwnd = 0
				}
				if handled {
					return ret
				}
				return hwnd.DefWindowProc(msg, wParam, lParam)
			},
		)
	})
	return windowProc
}

func nativeWindowStateFor(hwnd win.HWND, msg co.WM, lParam win.LPARAM) *nativeWindowState {
	if msg == co.WM_NCCREATE {
		create := (*win.CREATESTRUCT)(pointerFromUintptr(uintptr(lParam)))
		state := (*nativeWindowState)(pointerFromUintptr(create.LpCreateParams))
		if state == nil {
			return nil
		}
		state.hwnd = hwnd
		if _, err := hwnd.SetWindowLongPtr(
			co.GWLP_USERDATA,
			uintptr(unsafe.Pointer(state)),
		); err != nil {
			return nil
		}
		return state
	}

	ptr, err := hwnd.GetWindowLongPtr(co.GWLP_USERDATA)
	if err != nil || ptr == 0 {
		return nil
	}
	return (*nativeWindowState)(pointerFromUintptr(ptr))
}

// pointerFromUintptr round-trips pointer bits supplied by Win32 without doing
// pointer arithmetic on a uintptr, which would let the garbage collector move
// or reclaim the target between conversion steps.
func pointerFromUintptr(value uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&value))
}

func createNativeWindow(
	class win.ATOM,
	exStyle co.WS_EX,
	style co.WS,
	title string,
	position win.POINT,
	size win.SIZE,
	parent win.HWND,
	state *nativeWindowState,
) (win.HWND, error) {
	instance, err := currentInstance()
	if err != nil {
		return 0, err
	}
	hwnd, err := win.CreateWindowEx(
		exStyle,
		win.ClassNameAtom(class),
		title,
		style,
		position,
		size,
		parent,
		0,
		instance,
		win.LPARAM(unsafe.Pointer(state)),
	)
	if err != nil {
		return 0, fmt.Errorf("create Win32 window %q: %w", title, err)
	}
	return hwnd, nil
}

func runMessageLoop() (int, error) {
	var msg win.MSG
	for {
		ret, err := win.GetMessage(&msg, 0, 0, 0)
		if err != nil {
			return 1, fmt.Errorf("read Windows message: %w", err)
		}
		if ret == 0 {
			return int(msg.WParam), nil
		}
		if dispatchDocumentShortcut(&msg) {
			continue
		}
		win.TranslateMessage(&msg)
		win.DispatchMessage(&msg)
	}
}

func dpiFor(hwnd win.HWND) int {
	if hwnd != 0 {
		if dpi := hwnd.GetDpiForWindow(); dpi > 0 {
			return dpi
		}
	}
	if dpi := win.GetDpiForSystem(); dpi > 0 {
		return dpi
	}
	return 96
}

func clientSize(hwnd win.HWND) (width, height int, err error) {
	rect, err := hwnd.GetClientRect()
	if err != nil {
		return 0, 0, fmt.Errorf("get client rectangle for hwnd %#x: %w", uintptr(hwnd), err)
	}
	return int(rect.Right - rect.Left), int(rect.Bottom - rect.Top), nil
}
