package platform

import (
	"path/filepath"
	"testing"
)

func TestResolvePathsUsesPortableDataBesideExecutable(t *testing.T) {
	exeDir := filepath.Join(`C:\`, "Apps", "ClipNote")
	probedDir := ""

	got, err := ResolvePaths(exeDir, func(dir string) bool {
		probedDir = dir
		return true
	})
	if err != nil {
		t.Fatal(err)
	}

	if probedDir != exeDir {
		t.Fatalf("writability probe checked %q, want executable directory %q", probedDir, exeDir)
	}
	if want := filepath.Join(exeDir, "logs"); got.LogDir != want {
		t.Fatalf("log dir = %q, want %q", got.LogDir, want)
	}
	if want := filepath.Join(exeDir, "cache"); got.CacheDir != want {
		t.Fatalf("cache dir = %q, want %q", got.CacheDir, want)
	}
	if want := filepath.Join(exeDir, "config.json"); got.ConfigFile != want {
		t.Fatalf("config file = %q, want %q", got.ConfigFile, want)
	}
}

func TestResolvePathsRejectsUnwritableExecutableDirectory(t *testing.T) {
	exeDir := filepath.Join(`C:\`, "Program Files", "ClipNote")

	if _, err := ResolvePaths(exeDir, func(string) bool { return false }); err == nil {
		t.Fatal("expected an unwritable portable data directory to be rejected")
	}
}

func TestResolvePathsRejectsInvalidArguments(t *testing.T) {
	for _, test := range []struct {
		name     string
		exeDir   string
		canWrite func(string) bool
	}{
		{
			name:     "missing executable directory",
			canWrite: func(string) bool { return true },
		},
		{
			name:   "missing write probe",
			exeDir: `C:\Apps\ClipNote`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ResolvePaths(test.exeDir, test.canWrite); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
