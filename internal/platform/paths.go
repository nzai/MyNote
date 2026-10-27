package platform

import (
	"errors"
	"path/filepath"
)

type Paths struct {
	ConfigFile string
	CacheDir   string
	LogDir     string
}

func ResolvePaths(exeDir string, canWrite func(string) bool) (Paths, error) {
	if exeDir == "" {
		return Paths{}, errors.New("resolve paths: executable directory is empty")
	}
	if canWrite == nil {
		return Paths{}, errors.New("resolve paths: write probe is nil")
	}

	exeDir = filepath.Clean(exeDir)
	if !canWrite(exeDir) {
		return Paths{}, errors.New(
			"resolve paths: portable data directory " + exeDir + " is not writable",
		)
	}

	return Paths{
		ConfigFile: filepath.Join(exeDir, "config.json"),
		CacheDir:   filepath.Join(exeDir, "cache"),
		LogDir:     filepath.Join(exeDir, "logs"),
	}, nil
}
