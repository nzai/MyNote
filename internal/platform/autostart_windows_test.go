//go:build windows

package platform

import "testing"

func TestAutoStartCommandQuotesExecutablePath(t *testing.T) {
	got := autoStartCommand(`C:\Program Files\ClipNote\clipnote.exe`)
	want := `"C:\Program Files\ClipNote\clipnote.exe"`
	if got != want {
		t.Fatalf("autoStartCommand() = %q, want %q", got, want)
	}
}

func TestAutoStartCommandRejectsUnsafePath(t *testing.T) {
	for _, path := range []string{"", ` `, "clipnote.exe", `C:\bad"path\clipnote.exe`} {
		if got, err := safeAutoStartCommand(path); err == nil {
			t.Fatalf("safeAutoStartCommand(%q) = %q, want error", path, got)
		}
	}
}
