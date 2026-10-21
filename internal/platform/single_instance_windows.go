//go:build windows

package platform

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
	"github.com/rodrigocfd/windigo/wstr"
)

const (
	errorAlreadyExists syscall.Errno = 183
	notifyAttempts                   = 20
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutexW = kernel32.NewProc("CreateMutexW")
)

type InstanceDecision uint8

const (
	InstanceOwner InstanceDecision = iota
	InstanceSecondary
)

type SingleInstance struct {
	mu     sync.Mutex
	handle win.HANDLE
}

func AcquireSingleInstance(name string) (*SingleInstance, InstanceDecision, error) {
	if strings.TrimSpace(name) == "" {
		return nil, InstanceOwner, errors.New("acquire single instance: mutex name is empty")
	}

	var encoded wstr.BufEncoder
	ret, _, callErr := procCreateMutexW.Call(
		0,
		0,
		uintptr(encoded.AllowEmpty(name)),
	)
	if ret == 0 {
		return nil, InstanceOwner, fmt.Errorf("create single-instance mutex %q: %w", name, callErr)
	}

	decision := decideInstance(callErr == errorAlreadyExists)
	if decision == InstanceSecondary {
		handle := win.HANDLE(ret)
		if err := handle.CloseHandle(); err != nil {
			return nil, decision, fmt.Errorf("close secondary-instance mutex %q: %w", name, err)
		}
		return nil, decision, nil
	}
	return &SingleInstance{handle: win.HANDLE(ret)}, decision, nil
}

func decideInstance(alreadyExists bool) InstanceDecision {
	if alreadyExists {
		return InstanceSecondary
	}
	return InstanceOwner
}

func (s *SingleInstance) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle == 0 {
		return nil
	}
	handle := s.handle
	s.handle = 0
	if err := handle.CloseHandle(); err != nil {
		return fmt.Errorf("close single-instance mutex: %w", err)
	}
	return nil
}

func NotifyExistingInstance(windowClass string, message co.WM) error {
	if strings.TrimSpace(windowClass) == "" {
		return errors.New("notify existing instance: window class is empty")
	}
	for attempt := 0; attempt < notifyAttempts; attempt++ {
		hwnd, found := win.FindWindow(win.ClassNameStr(windowClass), "")
		if found {
			if err := hwnd.PostMessage(message, 0, 0); err != nil {
				return fmt.Errorf("post open signal to existing instance: %w", err)
			}
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf(
		"find existing instance window %q after %d attempts",
		windowClass,
		notifyAttempts,
	)
}
