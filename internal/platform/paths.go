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

func ResolvePaths(exeDir, localAppData string, canWrite func(string) bool) (Paths, error) {
	if exeDir == "" {
		return Paths{}, errors.New("resolve paths: executable directory is empty")
	}
	if localAppData == "" {
		return Paths{}, errors.New("resolve paths: local app data directory is empty")
	}
	if canWrite == nil {
		return Paths{}, errors.New("resolve paths: write probe is nil")
	}

	exeDir = filepath.Clean(exeDir)
	localAppData = filepath.Clean(localAppData)
	appDataDir := filepath.Join(localAppData, "ClipNote")

	logDir := filepath.Join(exeDir, "logs")
	if !canWrite(logDir) {
		logDir = filepath.Join(appDataDir, "logs")
	}

	return Paths{
		ConfigFile: filepath.Join(appDataDir, "config.json"),
		CacheDir:   filepath.Join(appDataDir, "cache"),
		LogDir:     logDir,
	}, nil
}
