//go:build windows

package platform

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/nzai/MyNote/internal/core"
	"github.com/nzai/MyNote/internal/save"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
	"github.com/rodrigocfd/windigo/x/cosh"
	"github.com/rodrigocfd/windigo/x/winsh"
)

type winSaveDialog struct {
	comOnce sync.Once
	comErr  error
}

var _ save.Dialog = (*winSaveDialog)(nil)

func NewWinSaveDialog() save.Dialog {
	return &winSaveDialog{}
}

func (d *winSaveDialog) Choose(ctx context.Context, options save.DialogOptions) (string, error) {
	if ctx == nil {
		return "", errors.New("show Win32 save dialog: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("show Win32 save dialog: %w", err)
	}
	filter, extension, err := saveDialogFilter(options.Kind)
	if err != nil {
		return "", fmt.Errorf("show Win32 save dialog: %w", err)
	}
	if err := d.initializeCOM(); err != nil {
		return "", err
	}

	releaser := win.NewOleReleaser()
	defer releaser.Release()

	var dialog *winsh.IFileSaveDialog
	if err := win.CoCreateInstance(
		releaser,
		&cosh.CLSID_FileSaveDialog,
		nil,
		co.CLSCTX_INPROC_SERVER,
		&dialog,
	); err != nil {
		return "", fmt.Errorf("create IFileSaveDialog: %w", err)
	}
	if options.Title != "" {
		if err := dialog.SetTitle(options.Title); err != nil {
			return "", fmt.Errorf("set save dialog title: %w", err)
		}
	}
	if err := dialog.SetFileTypes([]winsh.COMDLG_FILTERSPEC{filter}); err != nil {
		return "", fmt.Errorf("set save dialog file type filter: %w", err)
	}
	if err := dialog.SetFileTypeIndex(1); err != nil {
		return "", fmt.Errorf("select save dialog file type: %w", err)
	}
	if err := dialog.SetDefaultExtension(extension); err != nil {
		return "", fmt.Errorf("set save dialog default extension %q: %w", extension, err)
	}
	if options.SuggestedName != "" {
		if err := dialog.SetFileName(options.SuggestedName); err != nil {
			return "", fmt.Errorf("set save dialog suggested name: %w", err)
		}
	}
	currentOptions, err := dialog.GetOptions()
	if err != nil {
		return "", fmt.Errorf("read save dialog options: %w", err)
	}
	currentOptions |= cosh.FOS_FORCEFILESYSTEM | cosh.FOS_OVERWRITEPROMPT | cosh.FOS_PATHMUSTEXIST
	if err := dialog.SetOptions(currentOptions); err != nil {
		return "", fmt.Errorf("set save dialog options: %w", err)
	}

	ok, err := dialog.Show(win.HWND(options.Parent))
	if err != nil {
		if isSaveDialogCanceled(err) {
			return "", save.ErrCanceled
		}
		return "", fmt.Errorf("show Win32 save dialog: %w", err)
	}
	if !ok {
		return "", save.ErrCanceled
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("show Win32 save dialog: %w", err)
	}
	item, err := dialog.GetResult(releaser)
	if err != nil {
		return "", fmt.Errorf("get save dialog result: %w", err)
	}
	path, err := item.GetDisplayName(cosh.SIGDN_FILESYSPATH)
	if err != nil {
		return "", fmt.Errorf("resolve save dialog file system path: %w", err)
	}
	if path == "" {
		return "", save.ErrCanceled
	}
	return path, nil
}

func isSaveDialogCanceled(err error) bool {
	if errors.Is(err, co.ERROR_CANCELLED) {
		return true
	}
	// IFileDialog::Show returns HRESULT_FROM_WIN32(ERROR_CANCELLED), not the
	// bare Win32 error code, when the user closes the dialog.
	var hresult co.HRESULT
	return errors.As(err, &hresult) &&
		hresult == co.ERROR_CANCELLED.ToHresult()
}

func (d *winSaveDialog) initializeCOM() error {
	d.comOnce.Do(func() {
		_, err := win.CoInitializeEx(co.COINIT_APARTMENTTHREADED | co.COINIT_DISABLE_OLE1DDE)
		if err != nil {
			d.comErr = fmt.Errorf("initialize STA COM for save dialog: %w", err)
		}
	})
	return d.comErr
}

func saveDialogFilter(kind core.Kind) (winsh.COMDLG_FILTERSPEC, string, error) {
	switch kind {
	case core.KindText:
		return winsh.COMDLG_FILTERSPEC{Name: "文本文件 (*.txt)", Spec: "*.txt"}, "txt", nil
	case core.KindImage:
		return winsh.COMDLG_FILTERSPEC{Name: "PNG 图片 (*.png)", Spec: "*.png"}, "png", nil
	default:
		return winsh.COMDLG_FILTERSPEC{}, "", fmt.Errorf("unsupported save content kind %d", kind)
	}
}
