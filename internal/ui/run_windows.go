//go:build windows

package ui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nzai/MyNote/assets"
	"github.com/nzai/MyNote/internal/config"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

type Options struct {
	Reader         ClipboardReader
	ReaderFactory  ClipboardReaderFactory
	Cache          CacheEnqueuer
	CacheEvents    *CacheEventSink
	SaveFactory    SaveFactory
	OpenCacheDir   func() error
	ClearCache     func(context.Context) error
	ClearClipboard func(win.HWND) error
	EditorSettings config.EditorConfig
	AutoStart      bool
	SaveSettings   func(config.EditorConfig, bool) error
	ProgressDelay  time.Duration
	Logger         *slog.Logger
}

func Run(options Options) int {
	if err := win.SetProcessDpiAwarenessContext(
		co.DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2,
	); err != nil {
		// The process may already have inherited DPI awareness from a host.
		_ = win.SetProcessDPIAware()
	}
	icon, iconOwned, err := loadTrayIcon(options.Logger)
	if err != nil {
		showFatalUIError(err)
		return 1
	}
	controller, err := NewController(ControllerOptions{
		Reader:         options.Reader,
		ReaderFactory:  options.ReaderFactory,
		Cache:          options.Cache,
		CacheEvents:    options.CacheEvents,
		SaveFactory:    options.SaveFactory,
		OpenCacheDir:   options.OpenCacheDir,
		ClearCache:     options.ClearCache,
		ClearClipboard: options.ClearClipboard,
		Icon:           icon,
		EditorSettings: options.EditorSettings,
		AutoStart:      options.AutoStart,
		SaveSettings:   options.SaveSettings,
		ProgressDelay:  options.ProgressDelay,
		Logger:         options.Logger,
	})
	if err != nil {
		if iconOwned {
			_ = icon.DestroyIcon()
		}
		showFatalUIError(err)
		return 1
	}

	exitCode, runErr := runMessageLoop()
	if runErr != nil {
		controller.Close()
	}
	resourceErr := releaseUIResources()
	var iconErr error
	if iconOwned {
		iconErr = icon.DestroyIcon()
	}
	if err := errors.Join(runErr, resourceErr, iconErr); err != nil {
		showFatalUIError(err)
		return 1
	}
	return exitCode
}

func loadTrayIcon(logger *slog.Logger) (win.HICON, bool, error) {
	icon, err := loadAppIcon(assets.AppICO)
	if err == nil {
		return icon, true, nil
	}
	fallback, fallbackErr := loadStockAppIcon()
	if fallbackErr != nil {
		return 0, false, errors.Join(
			fmt.Errorf("load embedded app icon: %w", err),
			fallbackErr,
		)
	}
	if logger != nil {
		logger.Warn(
			"embedded tray icon unavailable; using stock application icon",
			"error", err,
		)
	}
	return fallback, false, nil
}

func showFatalUIError(err error) {
	if err == nil {
		return
	}
	_, _ = win.HWND(0).MessageBox(fmt.Sprintf("%v", err), "ClipNote 错误", co.MB_OK|co.MB_ICONERROR)
}

func ShowFatalError(err error) {
	showFatalUIError(err)
}
