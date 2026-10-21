package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nzai/MyNote/internal/cache"
	"github.com/nzai/MyNote/internal/clipboard"
	"github.com/nzai/MyNote/internal/config"
	"github.com/nzai/MyNote/internal/core"
	"github.com/nzai/MyNote/internal/logging"
	"github.com/nzai/MyNote/internal/platform"
	"github.com/nzai/MyNote/internal/save"
	"github.com/nzai/MyNote/internal/ui"
	"github.com/rodrigocfd/windigo/win"
)

const (
	singleInstanceName = `Local\ClipNote.SingleInstance`
	startupCleanupTime = 10 * time.Second
	workerCloseTime    = 3 * time.Second
)

var version = "0.1.0-dev"

type cliOptions struct {
	check       bool
	debugScopes []string
}

type appRuntime struct {
	cfg       config.Config
	paths     platform.Paths
	exePath   string
	logger    *logging.Factory
	debugList []string
}

func main() {
	runtime.LockOSThread()
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	options, err := parseCLI(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ClipNote: %v\n", err)
		return 2
	}

	app, err := initialize(options)
	if err != nil {
		reportStartupError(options.check, err)
		return 1
	}
	defer func() {
		if err := app.logger.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "ClipNote: close log: %v\n", err)
		}
	}()

	if options.check {
		app.logger.Scope("app").Info(
			"diagnostic check",
			"task_id", "check",
			"version", version,
			"executable", app.exePath,
			"log_dir", app.paths.LogDir,
			"log_file", app.logger.LogPath(),
			"cache_dir", app.paths.CacheDir,
			"config_file", app.paths.ConfigFile,
		)
		fmt.Fprint(os.Stdout, checkReport(app))
		return 0
	}

	return runApplication(app)
}

func parseCLI(args []string) (cliOptions, error) {
	flags := flag.NewFlagSet("ClipNote", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	check := flags.Bool("check", false, "print diagnostics and exit")
	debug := flags.String("debug", "", "comma-separated debug scopes")
	if err := flags.Parse(args); err != nil {
		return cliOptions{}, fmt.Errorf("parse command line: %w", err)
	}
	if flags.NArg() != 0 {
		return cliOptions{}, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	return cliOptions{
		check:       *check,
		debugScopes: mergeDebugScopes(nil, *debug),
	}, nil
}

func initialize(options cliOptions) (*appRuntime, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable path: %w", err)
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return nil, fmt.Errorf("normalize executable path %q: %w", exePath, err)
	}
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		localAppData, err = os.UserCacheDir()
		if err != nil {
			return nil, fmt.Errorf("resolve local app data directory: %w", err)
		}
	}
	paths, err := platform.ResolvePaths(filepath.Dir(exePath), localAppData, canWriteDir)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(paths.ConfigFile)
	if err != nil {
		return nil, err
	}
	debugScopes := mergeDebugScopes(cfg.Log.DebugScopes, strings.Join(options.debugScopes, ","))
	level, err := parseLogLevel(cfg.Log.Level)
	if err != nil {
		return nil, err
	}
	logger, err := logging.New(logging.Options{
		Dir:         paths.LogDir,
		Level:       level,
		DebugScopes: debugScopes,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize logging in %q: %w", paths.LogDir, err)
	}
	logger.Scope("app").Info(
		"ClipNote starting",
		"task_id", "startup",
		"version", version,
		"executable", exePath,
		"config_file", paths.ConfigFile,
		"cache_dir", paths.CacheDir,
		"log_dir", paths.LogDir,
		"log_file", logger.LogPath(),
		"debug_scopes", strings.Join(debugScopes, ","),
	)
	return &appRuntime{
		cfg:       cfg,
		paths:     paths,
		exePath:   exePath,
		logger:    logger,
		debugList: debugScopes,
	}, nil
}

func runApplication(app *appRuntime) int {
	logger := app.logger.Scope("app").With("task_id", "lifecycle")
	instance, decision, err := platform.AcquireSingleInstance(singleInstanceName)
	if err != nil {
		logger.Error("acquire single-instance mutex", "error", err)
		ui.ShowFatalError(err)
		return 1
	}
	if decision == platform.InstanceSecondary {
		if err := platform.NotifyExistingInstance(ui.ControllerWindowClassName, ui.OpenClipboardSignal); err != nil {
			logger.Error("notify running ClipNote instance", "error", err)
			ui.ShowFatalError(err)
			return 1
		}
		logger.Info("forwarded open request to the running instance")
		return 0
	}
	defer func() {
		if err := instance.Close(); err != nil {
			logger.Error("close single-instance mutex", "error", err)
		}
	}()

	cacheLogger := app.logger.Scope("cache").With("task_id", "cache-worker")
	store := cache.NewStore(cache.Options{
		Root:     app.paths.CacheDir,
		MaxAge:   time.Duration(app.cfg.Cache.MaxAgeDays) * 24 * time.Hour,
		MaxBytes: app.cfg.Cache.MaxBytes,
	})
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), startupCleanupTime)
	partials, partialErr := store.CleanupPartials(cleanupCtx)
	if partialErr != nil {
		cacheLogger.Warn("startup partial-cache cleanup failed", "error", partialErr)
	} else if partials > 0 {
		cacheLogger.Info("removed incomplete cache directories", "count", partials)
	}
	cleanup, err := store.Enforce(cleanupCtx)
	cleanupCancel()
	if err != nil {
		cacheLogger.Warn("initial cache retention failed", "error", err)
	} else {
		cacheLogger.Info(
			"initial cache retention completed",
			"removed", cleanup.Removed,
			"bytes_removed", cleanup.BytesRemoved,
			"remaining_bytes", cleanup.RemainingBytes,
			"invalid_records", len(cleanup.Invalid),
		)
	}

	cacheEvents := ui.NewCacheEventSink()
	worker := cache.NewWorker(store, func(event cache.Event) {
		if err := cacheEvents.Push(event); err != nil {
			if !errors.Is(err, ui.ErrCacheEventSinkClosed) {
				cacheLogger.Error(
					"deliver cache completion event",
					"capture_id", event.ID,
					"error", err,
				)
			}
			return
		}
		attrs := []any{
			"capture_id", event.ID,
			"duration", event.FinishedAt.Sub(event.StartedAt),
		}
		switch {
		case event.Err != nil && event.Canceled:
			attrs = append(attrs, "canceled", true)
			cacheLogger.Info("cache write canceled", attrs...)
		case event.Err != nil:
			attrs = append(attrs, "error", event.Err)
			cacheLogger.Error("cache write failed", attrs...)
		default:
			attrs = append(
				attrs,
				"bytes", event.Record.Bytes,
				"directory", event.Record.Directory,
			)
			cacheLogger.Info("cache write completed", attrs...)
		}
	})

	uiLogger := app.logger.Scope("ui").With("task_id", "ui")
	openCacheDir := func() error {
		return launchExplorer(cacheLogger, app.paths.CacheDir)
	}
	clearCache := func(ctx context.Context) error {
		started := time.Now()
		err := store.Clear(ctx)
		if err != nil {
			cacheLogger.Error("clear cache", "duration", time.Since(started), "error", err)
			return err
		}
		cacheLogger.Info("cache cleared", "duration", time.Since(started))
		return nil
	}

	exitCode := ui.Run(ui.Options{
		ReaderFactory: func(hwnd win.HWND) ui.ClipboardReader {
			return clipboard.NewReader(
				platform.NewWinClipboardOpener(hwnd),
				core.Limits{
					MaxTextBytes:  app.cfg.Capture.MaxTextBytes,
					MaxImageBytes: app.cfg.Capture.MaxImageBytes,
				},
				time.Now,
				rand.Reader,
			)
		},
		Cache:         worker,
		CacheEvents:   cacheEvents,
		SaveFactory:   newSaveService,
		OpenCacheDir:  openCacheDir,
		ClearCache:    clearCache,
		ProgressDelay: time.Duration(app.cfg.UI.ProgressDelayMS) * time.Millisecond,
		Logger:        uiLogger,
	})

	closeCtx, closeCancel := context.WithTimeout(context.Background(), workerCloseTime)
	err = worker.Close(closeCtx)
	closeCancel()
	if err != nil {
		cacheLogger.Error("close cache worker", "error", err)
		retryCtx, retryCancel := context.WithTimeout(context.Background(), time.Second)
		retryErr := worker.Close(retryCtx)
		retryCancel()
		if retryErr != nil {
			cacheLogger.Error("cache worker did not stop before shutdown", "error", retryErr)
		}
	}
	postCleanupCtx, postCleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
	partials, partialErr = store.CleanupPartials(postCleanupCtx)
	postCleanupCancel()
	if partialErr != nil {
		cacheLogger.Error("post-shutdown partial-cache cleanup failed", "error", partialErr)
	} else if partials > 0 {
		cacheLogger.Info("removed incomplete cache directories during shutdown", "count", partials)
	}
	return exitCode
}

func newSaveService(snapshot core.Snapshot) ui.DocumentSaveService {
	service := save.NewService(
		platform.NewWinSaveDialog(),
		save.NewAtomicWriter(),
		time.Now,
	)
	service.SetSnapshot(snapshot)
	return service
}

func launchExplorer(logger *slog.Logger, directory string) error {
	cmd := exec.Command("explorer.exe", directory)
	started := time.Now()
	if err := cmd.Start(); err != nil {
		logger.Error(
			"start explorer",
			"command", cmd.String(),
			"cwd", directory,
			"duration", time.Since(started),
			"error", err,
		)
		return fmt.Errorf("start %s: %w", cmd.String(), err)
	}
	logger.Debug(
		"start explorer",
		"command", cmd.String(),
		"cwd", directory,
		"pid", cmd.Process.Pid,
	)
	go func() {
		err := cmd.Wait()
		attrs := []any{
			"command", cmd.String(),
			"duration", time.Since(started),
		}
		if err == nil {
			logger.Debug("explorer exited", attrs...)
			return
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			attrs = append(attrs, "exit_code", exitErr.ExitCode())
		}
		attrs = append(attrs, "error", err)
		logger.Warn("explorer exited with error", attrs...)
	}()
	return nil
}

func canWriteDir(dir string) bool {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	file, err := os.CreateTemp(dir, ".clipnote-write-*")
	if err != nil {
		return false
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return false
	}
	return os.Remove(name) == nil
}

func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("parse log level: unsupported value %q", value)
	}
}

func mergeDebugScopes(configured []string, raw string) []string {
	result := make([]string, 0, len(configured)+4)
	seen := make(map[string]bool, len(configured)+4)
	add := func(value string) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		result = append(result, value)
	}
	for _, scope := range configured {
		add(scope)
	}
	for _, scope := range strings.Split(raw, ",") {
		add(scope)
	}
	return result
}

func checkReport(app *appRuntime) string {
	return fmt.Sprintf(
		"version=%s\nexecutable=%s\nlog_dir=%s\nlog_file=%s\ncache_dir=%s\nconfig_file=%s\nmax_text_bytes=%d\nmax_image_bytes=%d\ncache_max_age_days=%d\ncache_max_bytes=%d\nprogress_delay_ms=%d\ndebug_scopes=%s\n",
		version,
		app.exePath,
		app.paths.LogDir,
		app.logger.LogPath(),
		app.paths.CacheDir,
		app.paths.ConfigFile,
		app.cfg.Capture.MaxTextBytes,
		app.cfg.Capture.MaxImageBytes,
		app.cfg.Cache.MaxAgeDays,
		app.cfg.Cache.MaxBytes,
		app.cfg.UI.ProgressDelayMS,
		strings.Join(app.debugList, ","),
	)
}

func reportStartupError(check bool, err error) {
	if check {
		fmt.Fprintf(os.Stderr, "ClipNote check failed: %v\n", err)
		return
	}
	ui.ShowFatalError(err)
}
