//go:build windows

package ui

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nzai/MyNote/internal/core"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

const (
	cascadeStartDIP = 40
	cascadeStepDIP  = 24

	cacheClearTimeout = 30 * time.Second
)

type ClipboardReader interface {
	Read(context.Context) (core.Snapshot, error)
}

type CacheEnqueuer interface {
	Enqueue(context.Context, core.Snapshot) func()
}

type DocumentSaveService interface {
	SetText(string)
	Save(context.Context, uintptr) (string, error)
	SaveAs(context.Context, uintptr) (string, error)
}

type SaveFactory func(core.Snapshot) DocumentSaveService

type ControllerOptions struct {
	Reader       ClipboardReader
	Cache        CacheEnqueuer
	SaveFactory  SaveFactory
	OpenCacheDir func() error
	ClearCache   func(context.Context) error
	Icon         win.HICON
}

type Controller struct {
	native nativeWindowState
	tray   *Tray
	opts   ControllerOptions

	mu        sync.Mutex
	documents []*Document
	cascade   int
	cancels   map[string]func()
}

func NewController(options ControllerOptions) (*Controller, error) {
	if options.Icon == 0 {
		icon, err := loadStockAppIcon()
		if err != nil {
			return nil, err
		}
		options.Icon = icon
	}
	class, err := registerNativeClass(controllerWindowClass)
	if err != nil {
		return nil, err
	}
	controller := &Controller{
		opts:    options,
		cancels: make(map[string]func()),
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
	if c.opts.Reader == nil {
		return errors.New("打开剪贴板失败：剪贴板读取器未配置")
	}
	snapshot, err := c.opts.Reader.Read(context.Background())
	if err != nil {
		return fmt.Errorf("读取当前剪贴板：%w", err)
	}

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
		Snapshot: snapshot,
		Position: position,
		OnDestroy: func(destroyed *Document) {
			c.removeDocument(destroyed)
		},
	}
	if saveService != nil {
		options.Save = func(parent win.HWND, saveAs bool) (string, error) {
			saveService.SetText(document.SaveState().Text())
			if saveAs {
				return saveService.SaveAs(context.Background(), uintptr(parent))
			}
			return saveService.Save(context.Background(), uintptr(parent))
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
	document.Show()
	if c.opts.Cache != nil {
		cancel := c.opts.Cache.Enqueue(context.Background(), snapshot)
		if cancel != nil {
			c.mu.Lock()
			c.cancels[snapshot.CaptureID] = cancel
			c.mu.Unlock()
		}
	}
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
	case co.WM_COMMAND:
		c.handleTrayCommand(TrayCommand(uint16(wParam)))
		return 0, true
	case co.WM_CLOSE:
		c.closeDocuments()
		if err := c.native.hwnd.DestroyWindow(); err != nil {
			_, _ = c.native.hwnd.MessageBox(
				err.Error(),
				"退出 ClipNote 失败",
				co.MB_OK|co.MB_ICONERROR,
			)
		}
		return 0, true
	case co.WM_NCDESTROY:
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
			c.notifyError("菜单打开失败", err)
			return
		}
		if command != TrayCommandNone {
			if err := c.native.hwnd.PostMessage(co.WM_COMMAND, win.WPARAM(command), 0); err != nil {
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
			c.notifyError("无法打开缓存", errors.New("未配置缓存目录"))
			return
		}
		if err := c.opts.OpenCacheDir(); err != nil {
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
		c.notifyError("无法打开剪贴板", err)
	}
}

func (c *Controller) clearCache() {
	if c.opts.ClearCache == nil {
		c.notifyError("无法清空缓存", errors.New("未配置缓存存储"))
		return
	}
	answer, err := c.native.hwnd.MessageBox(
		"确定清空 ClipNote 的临时缓存吗？手动保存的文件不会受影响。",
		"清空临时缓存",
		co.MB_YESNO|co.MB_ICONWARNING|co.MB_DEFBUTTON2,
	)
	if err != nil {
		c.notifyError("确认清空缓存失败", err)
		return
	}
	if answer != co.ID_YES {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cacheClearTimeout)
	defer cancel()
	if err := c.opts.ClearCache(ctx); err != nil {
		c.notifyError("清空缓存失败", err)
		return
	}
	if err := c.tray.Notify("ClipNote", "临时缓存已清空。", false); err != nil {
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
	defer c.mu.Unlock()
	for index, existing := range c.documents {
		if existing == document {
			c.documents = append(c.documents[:index], c.documents[index+1:]...)
			break
		}
	}
	if len(c.documents) == 0 {
		c.cascade = 0
	}
}

func (c *Controller) closeDocuments() {
	c.mu.Lock()
	documents := append([]*Document(nil), c.documents...)
	c.mu.Unlock()
	for _, document := range documents {
		document.Close()
	}
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
