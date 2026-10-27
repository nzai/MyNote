//go:build windows

package platform

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

const (
	autoStartRunKey     = `Software\Microsoft\Windows\CurrentVersion\Run`
	autoStartValueName  = "ClipNote"
	autoStartCommandFmt = `"%s"`
)

type AutoStart interface {
	Enabled() (bool, error)
	SetEnabled(bool) error
}

type winAutoStart struct {
	command string
}

func NewWinAutoStart(exePath string) (AutoStart, error) {
	command, err := safeAutoStartCommand(exePath)
	if err != nil {
		return nil, err
	}
	return &winAutoStart{command: command}, nil
}

func autoStartCommand(exePath string) string {
	return fmt.Sprintf(autoStartCommandFmt, exePath)
}

func safeAutoStartCommand(exePath string) (string, error) {
	path := strings.TrimSpace(exePath)
	if path == "" {
		return "", errors.New("auto-start executable path is empty")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("auto-start executable path %q is not absolute", path)
	}
	if strings.ContainsRune(path, '"') {
		return "", fmt.Errorf("auto-start executable path %q contains a quote", path)
	}
	return autoStartCommand(path), nil
}

func (a *winAutoStart) Enabled() (bool, error) {
	key, err := win.HKEY_CURRENT_USER.RegOpenKeyEx(
		autoStartRunKey,
		co.REG_OPTION_NONE,
		co.KEY_QUERY_VALUE,
	)
	if errors.Is(err, co.ERROR_FILE_NOT_FOUND) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open auto-start registry key: %w", err)
	}
	defer func() {
		_ = key.RegCloseKey()
	}()

	value, err := key.RegQueryValueEx(autoStartValueName)
	if errors.Is(err, co.ERROR_FILE_NOT_FOUND) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read auto-start registry value: %w", err)
	}
	command, ok := value.Sz()
	if !ok {
		return false, nil
	}
	return command == a.command, nil
}

func (a *winAutoStart) SetEnabled(enabled bool) error {
	if !enabled {
		return a.disable()
	}
	key, _, err := win.HKEY_CURRENT_USER.RegCreateKeyEx(
		autoStartRunKey,
		"",
		co.REG_OPTION_NON_VOLATILE,
		co.KEY_SET_VALUE,
		nil,
	)
	if err != nil {
		return fmt.Errorf("open auto-start registry key for writing: %w", err)
	}
	defer func() {
		_ = key.RegCloseKey()
	}()
	if err := key.RegSetValueEx(autoStartValueName, win.RegValSz(a.command)); err != nil {
		return fmt.Errorf("write auto-start registry value: %w", err)
	}
	return nil
}

func (a *winAutoStart) disable() error {
	key, err := win.HKEY_CURRENT_USER.RegOpenKeyEx(
		autoStartRunKey,
		co.REG_OPTION_NONE,
		co.KEY_SET_VALUE,
	)
	if errors.Is(err, co.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open auto-start registry key for removal: %w", err)
	}
	defer func() {
		_ = key.RegCloseKey()
	}()
	if err := key.RegDeleteValue(autoStartValueName); errors.Is(err, co.ERROR_FILE_NOT_FOUND) {
		return nil
	} else if err != nil {
		return fmt.Errorf("remove auto-start registry value: %w", err)
	}
	return nil
}
