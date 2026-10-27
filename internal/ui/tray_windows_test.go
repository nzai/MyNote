//go:build windows

package ui

import (
	"reflect"
	"testing"
)

func TestTrayMenuUsesRequestedGroups(t *testing.T) {
	want := []trayMenuItem{
		{command: TrayCommandOpen, label: "新建并粘贴"},
		{},
		{command: TrayCommandClearClipboard, label: "清空剪贴板"},
		{},
		{command: TrayCommandOpenCache, label: "打开缓存文件夹"},
		{command: TrayCommandClearCache, label: "清空缓存"},
		{},
		{command: TrayCommandExit, label: "退出"},
	}

	if got := trayMenuItems(); !reflect.DeepEqual(got, want) {
		t.Fatalf("tray menu = %#v, want %#v", got, want)
	}
}
