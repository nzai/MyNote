//go:build windows

package ui

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
	"github.com/rodrigocfd/windigo/wstr"
)

const (
	trayCallbackMessage = co.WM_APP + 100

	nimAdd        = 0
	nimModify     = 1
	nimDelete     = 2
	nimSetVersion = 4

	nifMessage = 0x01
	nifIcon    = 0x02
	nifTip     = 0x04
	nifInfo    = 0x10
	nifShowTip = 0x80

	notifyIconVersion4 = 4

	ninSelect    = 0x0400
	ninKeySelect = 0x0401

	niiInfo  = 0x0000_0001
	niiError = 0x0000_0003
)

var (
	shell32              = syscall.NewLazyDLL("shell32.dll")
	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	user32               = syscall.NewLazyDLL("user32.dll")
	procAppendMenuW      = user32.NewProc("AppendMenuW")
)

type trayNotifyIconData struct {
	CbSize            uint32
	HWnd              win.HWND
	UID               uint32
	UFlags            uint32
	UCallbackMessage  uint32
	HIcon             win.HICON
	SzTip             [128]uint16
	DwState           uint32
	DwStateMask       uint32
	SzInfo            [256]uint16
	UTimeoutOrVersion uint32
	SzInfoTitle       [64]uint16
	DwInfoFlags       uint32
	GuidItem          [16]byte
	HBalloonIcon      win.HICON
}

type trayAction uint8

const (
	trayActionNone trayAction = iota
	trayActionOpen
	trayActionMenu
)

type trayMenuItem struct {
	command TrayCommand
	label   string
}

func trayMenuItems() []trayMenuItem {
	return []trayMenuItem{
		{command: TrayCommandOpen, label: "新建并粘贴"},
		{},
		{command: TrayCommandClearClipboard, label: "清空剪贴板"},
		{},
		{command: TrayCommandOpenCache, label: "打开缓存文件夹"},
		{command: TrayCommandClearCache, label: "清空缓存"},
		{},
		{command: TrayCommandExit, label: "退出"},
	}
}

type Tray struct {
	owner win.HWND
	icon  win.HICON
	added bool
}

func NewTray(owner win.HWND, icon win.HICON) (*Tray, error) {
	if owner == 0 {
		return nil, fmt.Errorf("create tray icon: owner window is null")
	}
	if icon == 0 {
		return nil, fmt.Errorf("create tray icon: icon handle is null")
	}
	tray := &Tray{owner: owner, icon: icon}
	data := tray.data()
	data.UFlags = nifMessage | nifIcon | nifTip
	data.UCallbackMessage = uint32(trayCallbackMessage)
	if err := shellNotifyIcon(nimAdd, &data); err != nil {
		return nil, fmt.Errorf(
			"add tray icon for hwnd %#x icon %#x: %w",
			uintptr(owner),
			uintptr(icon),
			err,
		)
	}
	tray.added = true

	version := tray.data()
	version.UTimeoutOrVersion = notifyIconVersion4
	if err := shellNotifyIcon(nimSetVersion, &version); err != nil {
		_ = tray.Close()
		return nil, fmt.Errorf("set tray icon version 4: %w", err)
	}
	data.UFlags |= nifShowTip
	if err := shellNotifyIcon(nimModify, &data); err != nil {
		_ = tray.Close()
		return nil, fmt.Errorf("enable modern tray tooltip: %w", err)
	}
	return tray, nil
}

func (t *Tray) Close() error {
	if t == nil || !t.added {
		return nil
	}
	data := t.data()
	if err := shellNotifyIcon(nimDelete, &data); err != nil {
		return fmt.Errorf("delete tray icon: %w", err)
	}
	t.added = false
	return nil
}

func (t *Tray) data() trayNotifyIconData {
	data := trayNotifyIconData{
		HWnd:  t.owner,
		UID:   1,
		HIcon: t.icon,
	}
	data.CbSize = uint32(unsafe.Sizeof(data))
	wstr.EncodeToBuf(data.SzTip[:], "ClipNote - 点击打开当前剪贴板")
	return data
}

func trayActionFor(lParam win.LPARAM) trayAction {
	event := uint32(lParam) & 0xffff
	switch event {
	case ninSelect, ninKeySelect:
		return trayActionOpen
	case uint32(co.WM_RBUTTONUP):
		return trayActionMenu
	default:
		return trayActionNone
	}
}

func shellTrayAvailable() bool {
	_, found := win.FindWindow(win.ClassNameStr("Shell_TrayWnd"), "")
	return found
}

func (t *Tray) ShowMenu() (TrayCommand, error) {
	menu, err := win.CreatePopupMenu()
	if err != nil {
		return TrayCommandNone, fmt.Errorf("create tray menu: %w", err)
	}
	defer func() {
		_ = menu.DestroyMenu()
	}()

	items := trayMenuItems()
	for _, item := range items {
		flags := uintptr(co.MF_STRING)
		commandID := uintptr(item.command)
		var encoded wstr.BufEncoder
		var label uintptr
		if item.label == "" {
			flags = uintptr(co.MF_SEPARATOR)
			commandID = 0
		} else {
			label = uintptr(encoded.AllowEmpty(item.label))
		}
		if err := appendMenu(menu, flags, commandID, label); err != nil {
			return TrayCommandNone, fmt.Errorf("add tray menu item %q: %w", item.label, err)
		}
	}

	cursor, err := win.GetCursorPos()
	if err != nil {
		return TrayCommandNone, fmt.Errorf("get cursor position for tray menu: %w", err)
	}
	t.owner.SetForegroundWindow()
	command, err := menu.TrackPopupMenu(
		co.TPM_RETURNCMD|co.TPM_RIGHTBUTTON,
		int(cursor.X),
		int(cursor.Y),
		t.owner,
	)
	_ = t.owner.PostMessage(co.WM_NULL, 0, 0)
	if err != nil {
		return TrayCommandNone, fmt.Errorf("track tray menu: %w", err)
	}
	return TrayCommand(command), nil
}

func (t *Tray) Notify(title, message string, isError bool) error {
	if t == nil || !t.added {
		return fmt.Errorf("show tray notification: tray icon is not active")
	}
	data := t.data()
	data.UFlags = nifInfo | nifShowTip
	wstr.EncodeToBuf(data.SzInfoTitle[:], title)
	wstr.EncodeToBuf(data.SzInfo[:], message)
	data.DwInfoFlags = niiInfo
	if isError {
		data.DwInfoFlags = niiError
	}
	if err := shellNotifyIcon(nimModify, &data); err != nil {
		return fmt.Errorf("show tray notification %q: %w", title, err)
	}
	return nil
}

func shellNotifyIcon(operation uint32, data *trayNotifyIconData) error {
	ret, _, callErr := procShellNotifyIconW.Call(
		uintptr(operation),
		uintptr(unsafe.Pointer(data)),
	)
	if ret == 0 {
		if callErr != nil && callErr != syscall.Errno(0) {
			return callErr
		}
		return co.ERROR_UNIDENTIFIED_ERROR
	}
	return nil
}

func appendMenu(menu win.HMENU, flags, commandID, label uintptr) error {
	ret, _, callErr := procAppendMenuW.Call(
		uintptr(menu),
		flags,
		commandID,
		label,
	)
	if ret == 0 {
		if callErr != nil && callErr != syscall.Errno(0) {
			return callErr
		}
		return co.ERROR_UNIDENTIFIED_ERROR
	}
	return nil
}
