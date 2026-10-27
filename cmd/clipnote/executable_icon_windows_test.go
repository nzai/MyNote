//go:build windows

package main

import (
	"fmt"
	"os"
	"syscall"
	"testing"
	"unsafe"
)

var extractIconExW = syscall.NewLazyDLL("shell32.dll").NewProc("ExtractIconExW")

func TestExecutableEmbedsApplicationIcon(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	count, err := executableIconGroupCount(executable)
	if err != nil {
		t.Fatalf("count icons in %q: %v", executable, err)
	}
	if count == 0 {
		t.Fatalf("%q has no embedded Windows icon resource", executable)
	}
}

func executableIconGroupCount(path string) (uint32, error) {
	pathUTF16, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, fmt.Errorf("encode executable path: %w", err)
	}
	result, _, callErr := extractIconExW.Call(
		uintptr(unsafe.Pointer(pathUTF16)),
		uintptr(0xffffffff),
		0,
		0,
		0,
	)
	if result == 0 && callErr != syscall.Errno(0) {
		return 0, fmt.Errorf("ExtractIconExW: %w", callErr)
	}
	return uint32(result), nil
}
