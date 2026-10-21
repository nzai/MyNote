package platform

import (
	"path/filepath"
	"testing"
)

func TestResolvePathsUsesExecutableLogsWhenWritable(t *testing.T) {
	exeDir := filepath.Join(`C:\`, "Apps", "ClipNote")
	localAppData := filepath.Join(`C:\`, "Users", "me", "AppData", "Local")

	got, err := ResolvePaths(exeDir, localAppData, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}

	if want := filepath.Join(exeDir, "logs"); got.LogDir != want {
		t.Fatalf("log dir = %q, want %q", got.LogDir, want)
	}
	if want := filepath.Join(localAppData, "ClipNote", "cache"); got.CacheDir != want {
		t.Fatalf("cache dir = %q, want %q", got.CacheDir, want)
	}
	if want := filepath.Join(localAppData, "ClipNote", "config.json"); got.ConfigFile != want {
		t.Fatalf("config file = %q, want %q", got.ConfigFile, want)
	}
}

func TestResolvePathsFallsBackWhenExecutableLogsAreNotWritable(t *testing.T) {
	exeDir := filepath.Join(`C:\`, "Program Files", "ClipNote")
	localAppData := filepath.Join(`C:\`, "Users", "me", "AppData", "Local")

	got, err := ResolvePaths(exeDir, localAppData, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}

	if want := filepath.Join(localAppData, "ClipNote", "logs"); got.LogDir != want {
		t.Fatalf("log dir = %q, want %q", got.LogDir, want)
	}
}

func TestResolvePathsRejectsMissingBaseDirectories(t *testing.T) {
	for _, test := range []struct {
		name         string
		exeDir       string
		localAppData string
	}{
		{name: "missing executable directory", localAppData: `C:\Users\me\AppData\Local`},
		{name: "missing local app data", exeDir: `C:\Apps\ClipNote`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ResolvePaths(test.exeDir, test.localAppData, func(string) bool { return true }); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
