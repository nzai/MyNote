//go:build windows

package ui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nzai/MyNote/internal/core"
	"github.com/nzai/MyNote/internal/save"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

const (
	cascadeStartDIP = 40
	cascadeStepDIP  = 24

	cacheClearTimeout    = 30 * time.Second
	defaultProgressDelay = 3 * time.Second
	OpenClipboardSignal  = co.WM_APP + 987
)

type ClipboardReader interface {
	Read(context.Context) (core.Snapshot, error)
}

type ClipboardReaderFactory func(win.HWND) ClipboardReader

type CacheEnqueuer interface {
	Enqueue(context.Context, core.Snapshot) func()
}

type DocumentSaveService interface {
	SetText(string)
	Prepare(context.Context, uintptr, bool) (save.Prepared, error)
	WritePrepared(context.Context, save.Prepared) error
	CommitPrepared(save.Prepared)
}

type SaveFactory func(core.Snapshot) DocumentSaveService

type ControllerOptions struct {
	Reader        ClipboardReader
	ReaderFactory ClipboardReaderFactory
	Cache         CacheEnqueuer
	CacheEvents   *CacheEventSink
	SaveFactory   SaveFactory
	OpenCacheDir  func() error
	ClearCache    func(context.Context) error
	Icon          win.HICON
	ProgressDelay time.Duration
	Logger        *slog.Logger
}

type Controller struct {
	native nativeWindowState
	tray   *Tray
	opts   ControllerOptions
	reader ClipboardReader

	mu             sync.Mutex
	documents      []*Document
	cascade        int
	progress       map[string]*cacheProgress
	nextProgressID int
	exit           exitCoordinator
}

type cacheProgress struct {
	document *Document
	cancel   func()
	timerID  int
	state    progressState
}

func NewController(options ControllerOptions) (*Controller, error) {
	if options.Icon == 0 {
		icon, err := loadStockAppIcon()
		if err != nil {
			return nil, err
		}
		options.Icon = icon
	}
	if options.ProgressDelay <= 0 {
		options.ProgressDelay = defaultProgressDelay
	}
	class, err := registerNativeClass(controllerWindowClass)
	if err != nil {
		return nil, err
	}
	controller := &Controller{
		opts:           options,
		progress:       make(map[string]*cacheProgress),
		nextProgressID: 1,
	}
	controller.native.handler = controller
	hwnd, err := createNativeWindow(
		class,
		0,
		co.WS_POPUP,
		"ClipNote.Controller",
		win.POINT{},
		win.SIZE{},
		0,
		&controller.native,
	)
	if err != nil {
		return nil, err
	}
	controller.native.hwnd = hwnd
	if options.CacheEvents != nil {
		if err := options.CacheEvents.Bind(hwnd); err != nil {
			_ = hwnd.DestroyWindow()
			return nil, err
		}
	}

	tray, err := NewTray(hwnd, options.Icon)
	if err != nil {
		_ = hwnd.DestroyWindow()
		return nil, err
	}
	controller.tray = tray
	return controller, nil
}

func (c *Controller) Hwnd() win.HWND {
	return c.native.hwnd
}

func (c *Controller) Show() {
	// The controller intentionally has no visible surface.
}

func (c *Controller) Close() {
	if c.native.hwnd == 0 {
		return
	}
	_, _ = c.native.hwnd.SendMessage(co.WM_CLOSE, 0, 0)
}

func (c *Controller) OpenCurrentClipboard() error {
	reader := c.opts.Reader
	if reader == nil {
		reader = c.reader
	}
	if reader == nil && c.opts.ReaderFactory != nil {
		reader = c.opts.ReaderFactory(c.native.hwnd)
		c.reader = reader
	}
	if reader == nil {
		return errors.New("打开剪贴板失败：剪贴板读取器未配置")
	}
	readStarted := time.Now()
	snapshot, err := reader.Read(context.Background())
	if err != nil {
		return fmt.Errorf("读取当前剪贴板：%w", err)
	}
	c.logger().Debug(
		"clipboard snapshot captured",
		"capture_id", snapshot.CaptureID,
		"kind", snapshot.Kind,
		"format", snapshot.SourceFormat,
		"bytes", snapshotBytes(snapshot),
		"duration", time.Since(readStarted),
	)

	c.mu.Lock()
	if len(c.documents) == 0 {
		c.cascade = 0
	}
	index := c.cascade
	c.cascade++
	c.mu.Unlock()

	position, err := c.documentPosition(index)
	if err != nil {
		return err
	}

	var document *Document
	var saveService DocumentSaveService
	if c.opts.SaveFactory != nil {
		saveService = c.opts.SaveFactory(snapshot)
	}
	options := DocumentOptions{
		Snapshot:      snapshot,
		Position:      position,
		Logger:        c.opts.Logger,
		ProgressDelay: c.opts.ProgressDelay,
		OnDestroy: func(destroyed *Document) {
			c.removeDocument(destroyed)
		},
		OnCloseCanceled: func(*Document) {
			c.cancelExit()
		},
		OnCancelCache: func() {
			c.cancelCache(snapshot.CaptureID)
		},
	}
	if saveService != nil {
		options.Save = func(parent win.HWND, saveAs bool) (DocumentSaveJob, error) {
			text := document.SaveState().Text()
			saveService.SetText(text)
			prepared, err := saveService.Prepare(context.Background(), uintptr(parent), saveAs)
			if err != nil {
				return DocumentSaveJob{}, err
			}
			return DocumentSaveJob{
				Run: func(ctx context.Context) (DocumentSaveResult, error) {
					if err := saveService.WritePrepared(ctx, prepared); err != nil {
						return DocumentSaveResult{}, err
					}
					saveService.CommitPrepared(prepared)
					return DocumentSaveResult{
						Path: prepared.Path(),
						Text: text,
					}, nil
				},
			}, nil
		}
	}
	document, err = NewDocument(options)
	if err != nil {
		return fmt.Errorf("创建剪贴板窗口：%w", err)
	}

	c.mu.Lock()
	c.documents = append(c.documents, document)
	c.mu.Unlock()

	// Showing and painting come first; the cache queue is deliberately touched
	// only after Show returns so disk I/O can never gate the first frame.
	showStarted := time.Now()
	document.Show()
	c.logger().Debug(
		"document window shown",
		"capture_id", snapshot.CaptureID,
		"kind", snapshot.Kind,
		"window_show_duration", time.Since(showStarted),
		"open_duration", time.Since(readStarted),
	)
	c.startCache(snapshot, document)
	return nil
}

func (c *Controller) handleNativeMessage(
	msg co.WM,
	wParam win.WPARAM,
	lParam win.LPARAM,
) (uintptr, bool) {
	switch msg {
	case trayCallbackMessage:
		c.handleTrayCallback(lParam)
		return 0, true
	case cacheEventMessage:
		c.handleCacheEvents()
		return 0, true
	case co.WM_TIMER:
		c.handleProgressTimer(int(wParam))
		return 0, true
	case OpenClipboardSignal:
		c.openClipboardFromTray()
		return 0, true
	case co.WM_COMMAND:
		c.handleTrayCommand(TrayCommand(uint16(wParam)))
		return 0, true
	case co.WM_CLOSE:
		if !c.exit.closeBatch(c.closeDocuments()) {
			return 0, true
		}
		c.cancelAllCache()
		if err := c.native.hwnd.DestroyWindow(); err != nil {
			c.logError("destroy controller window", err)
			_, _ = c.native.hwnd.MessageBox(
				err.Error(),
				"退出 ClipNote 失败",
				co.MB_OK|co.MB_ICONERROR,
			)
		}
		return 0, true
	case co.WM_NCDESTROY:
		c.cancelAllCache()
		if c.opts.CacheEvents != nil {
			c.opts.CacheEvents.Close()
		}
		if c.tray != nil {
			// Windows removes stale tray icons when the process exits, so a
			// failure here must not block shutdown.
			_ = c.tray.Close()
			c.tray = nil
		}
		win.PostQuitMessage(0)
	}
	return 0, false
}

func (c *Controller) handleTrayCallback(lParam win.LPARAM) {
	switch trayActionFor(lParam) {
	case trayActionOpen:
		c.openClipboardFromTray()
	case trayActionMenu:
		command, err := c.tray.ShowMenu()
		if err != nil {
			c.logError("show tray menu", err)
			c.notifyError("菜单打开失败", err)
			return
		}
		if command != TrayCommandNone {
			if err := c.native.hwnd.PostMessage(co.WM_COMMAND, win.WPARAM(command), 0); err != nil {
				c.logError("dispatch tray command", err)
				c.notifyError("托盘命令分发失败", err)
			}
		}
	}
}

func (c *Controller) handleTrayCommand(command TrayCommand) {
	switch command {
	case TrayCommandOpen:
		c.openClipboardFromTray()
	case TrayCommandOpenCache:
		if c.opts.OpenCacheDir == nil {
			c.logError("open cache directory", errors.New("cache directory callback is not configured"))
			c.notifyError("无法打开缓存", errors.New("未配置缓存目录"))
			return
		}
		if err := c.opts.OpenCacheDir(); err != nil {
			c.logError("open cache directory", err)
			c.notifyError("无法打开缓存目录", err)
		}
	case TrayCommandClearCache:
		c.clearCache()
	case TrayCommandExit:
		_ = c.native.hwnd.PostMessage(co.WM_CLOSE, 0, 0)
	}
}

func (c *Controller) openClipboardFromTray() {
	if err := c.OpenCurrentClipboard(); err != nil {
		c.logError("open current clipboard", err)
		c.notifyError("无法打开剪贴板", err)
	}
}

func (c *Controller) clearCache() {
	if c.opts.ClearCache == nil {
		c.logError("clear cache", errors.New("cache store callback is not configured"))
		c.notifyError("无法清空缓存", errors.New("未配置缓存存储"))
		return
	}
	answer, err := c.native.hwnd.MessageBox(
		"确定清空 ClipNote 的临时缓存吗？手动保存的文件不会受影响。",
		"清空临时缓存",
		co.MB_YESNO|co.MB_ICONWARNING|co.MB_DEFBUTTON2,
	)
	if err != nil {
		c.logError("confirm clear cache", err)
		c.notifyError("确认清空缓存失败", err)
		return
	}
	if answer != co.ID_YES {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cacheClearTimeout)
	defer cancel()
	if err := c.opts.ClearCache(ctx); err != nil {
		c.logError("clear cache", err)
		c.notifyError("清空缓存失败", err)
		return
	}
	if err := c.tray.Notify("ClipNote", "临时缓存已清空。", false); err != nil {
		c.logError("show cache-cleared notification", err)
		c.notifyError("显示清空结果失败", err)
	}
}

func (c *Controller) notifyError(title string, err error) {
	if c.tray == nil {
		return
	}
	_ = c.tray.Notify(title, err.Error(), true)
}

func (c *Controller) removeDocument(document *Document) {
	c.mu.Lock()
	for index, existing := range c.documents {
		if existing == document {
			c.documents = append(c.documents[:index], c.documents[index+1:]...)
			break
		}
	}
	if len(c.documents) == 0 {
		c.cascade = 0
	}
	for captureID, entry := range c.progress {
		if entry.document != document {
			continue
		}
		c.killProgressTimer(entry)
		delete(c.progress, captureID)
	}
	exitNow := c.exit.documentDestroyed(len(c.documents))
	c.mu.Unlock()

	if exitNow {
		_ = c.native.hwnd.PostMessage(co.WM_CLOSE, 0, 0)
	}
}

func (c *Controller) closeDocuments() []documentCloseDisposition {
	c.mu.Lock()
	documents := append([]*Document(nil), c.documents...)
	c.mu.Unlock()
	results := make([]documentCloseDisposition, 0, len(documents))
	for _, document := range documents {
		results = append(results, document.requestClose())
	}
	return results
}

func (c *Controller) cancelExit() {
	c.mu.Lock()
	c.exit.cancel()
	c.mu.Unlock()
}

func (c *Controller) documentPosition(index int) (win.POINT, error) {
	cursor, err := win.GetCursorPos()
	if err != nil {
		return win.POINT{}, fmt.Errorf("获取鼠标位置以放置剪贴板窗口：%w", err)
	}
	monitor := win.MonitorFromPoint(cursor, co.MONITOR_DEFAULTTONEAREST)
	info, err := monitor.GetMonitorInfo()
	if err != nil {
		return win.POINT{}, fmt.Errorf("获取显示器工作区：%w", err)
	}
	dpi := dpiFor(c.native.hwnd)
	width := scaleDIP(windowWidthDIP, dpi)
	height := scaleDIP(windowHeightDIP, dpi)
	return cascadePosition(info.RcWork, width, height, dpi, index), nil
}

func cascadePosition(work win.RECT, width, height, dpi, index int) win.POINT {
	if index < 0 {
		index = 0
	}
	margin := scaleDIP(cascadeStartDIP, dpi)
	step := scaleDIP(cascadeStepDIP, dpi)
	if step <= 0 {
		step = 1
	}

	maxLeft := int(work.Right) - width
	maxTop := int(work.Bottom) - height
	startX := int(work.Left) + margin
	startY := int(work.Top) + margin
	if startX > maxLeft {
		startX = maxLeft
	}
	if startY > maxTop {
		startY = maxTop
	}
	if startX < int(work.Left) {
		startX = int(work.Left)
	}
	if startY < int(work.Top) {
		startY = int(work.Top)
	}

	maxStartX := maxLeft - margin
	maxStartY := maxTop - margin
	if maxStartX < startX {
		maxStartX = startX
	}
	if maxStartY < startY {
		maxStartY = startY
	}
	stepsX := (maxStartX - startX) / step
	stepsY := (maxStartY - startY) / step
	steps := min(stepsX, stepsY)
	index %= steps + 1
	return win.POINT{
		X: int32(startX + step*index),
		Y: int32(startY + step*index),
	}
}

func (c *Controller) startCache(snapshot core.Snapshot, document *Document) {
	if c.opts.Cache == nil {
		document.SetCacheStatus("未启用缓存")
		return
	}

	cancel := c.opts.Cache.Enqueue(context.Background(), snapshot)
	entry := &cacheProgress{
		document: document,
		cancel:   cancel,
		state:    newProgressState(c.opts.ProgressDelay),
	}
	timerID := c.nextProgressID
	c.nextProgressID++
	if c.nextProgressID <= 0 {
		c.nextProgressID = 1
	}
	entry.timerID = timerID
	entry.state.Start(time.Now())
	c.progress[snapshot.CaptureID] = entry

	delayMS := int(c.opts.ProgressDelay / time.Millisecond)
	if delayMS < 1 {
		delayMS = 1
	}
	if err := c.native.hwnd.SetTimer(timerID, delayMS); err != nil {
		c.logError("schedule cache progress", err)
		entry.timerID = 0
		entry.state.Finish()
		document.SetCacheProgress("正在写入缓存…")
	}
}

func (c *Controller) handleProgressTimer(timerID int) {
	var entry *cacheProgress
	for _, candidate := range c.progress {
		if candidate.timerID == timerID {
			entry = candidate
			break
		}
	}
	if entry == nil {
		_ = c.native.hwnd.KillTimer(timerID)
		return
	}
	_ = c.native.hwnd.KillTimer(timerID)
	entry.timerID = 0
	if !entry.state.Visible(time.Now()) {
		remaining := entry.state.delay - time.Since(entry.state.started)
		if remaining < time.Millisecond {
			remaining = time.Millisecond
		}
		if err := c.native.hwnd.SetTimer(timerID, int(remaining/time.Millisecond)); err != nil {
			c.logError("reschedule cache progress", err)
			entry.state.Finish()
			entry.document.SetCacheProgress("正在写入缓存…")
		} else {
			entry.timerID = timerID
		}
		return
	}
	entry.state.Finish()
	entry.document.SetCacheProgress("正在写入缓存…")
}

func (c *Controller) handleCacheEvents() {
	if c.opts.CacheEvents == nil {
		return
	}
	for _, event := range c.opts.CacheEvents.Drain() {
		entry := c.progress[event.ID]
		if entry == nil {
			continue
		}
		c.killProgressTimer(entry)
		delete(c.progress, event.ID)

		switch {
		case event.Err != nil && event.Canceled:
			entry.document.SetCacheStatus("缓存已取消")
		case event.Err != nil:
			c.logger().Error(
				"cache clipboard snapshot",
				"capture_id", event.ID,
				"kind", entry.document.snapshot.Kind,
				"bytes", snapshotBytes(entry.document.snapshot),
				"duration", event.FinishedAt.Sub(event.StartedAt),
				"error", event.Err,
			)
			entry.document.SetCacheStatus("缓存失败，详见日志")
		default:
			entry.document.SetCacheStatus("已缓存")
		}
	}
}

func (c *Controller) cancelCache(captureID string) {
	entry := c.progress[captureID]
	if entry == nil || entry.cancel == nil {
		return
	}
	cancel := entry.cancel
	entry.cancel = nil
	cancel()
}

func (c *Controller) cancelAllCache() {
	for captureID, entry := range c.progress {
		c.killProgressTimer(entry)
		if entry.cancel != nil {
			entry.cancel()
		}
		delete(c.progress, captureID)
	}
}

func (c *Controller) killProgressTimer(entry *cacheProgress) {
	if entry != nil && entry.timerID != 0 {
		_ = c.native.hwnd.KillTimer(entry.timerID)
		entry.timerID = 0
	}
}

func (c *Controller) logger() *slog.Logger {
	if c.opts.Logger == nil {
		return slog.Default()
	}
	return c.opts.Logger
}

func (c *Controller) logError(message string, err error) {
	c.logger().Error(message, "error", err)
}

func snapshotBytes(snapshot core.Snapshot) int64 {
	if snapshot.Kind == core.KindText {
		return int64(len(snapshot.Text))
	}
	return int64(len(snapshot.DIB))
}
