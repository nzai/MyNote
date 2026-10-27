package main

import (
	"errors"
	"fmt"

	"github.com/nzai/MyNote/internal/config"
)

func persistUISettings(
	cfg config.Config,
	editor config.EditorConfig,
	autoStart bool,
	currentAutoStart bool,
	setAutoStart func(bool) error,
	saveConfig func(config.Config) error,
) (config.Config, error) {
	if err := editor.Validate(); err != nil {
		return cfg, fmt.Errorf("validate editor settings: %w", err)
	}
	next := cfg
	next.UI.Editor = editor
	autoStartChanged := autoStart != currentAutoStart
	if autoStartChanged {
		if err := setAutoStart(autoStart); err != nil {
			return cfg, fmt.Errorf("update auto-start setting: %w", err)
		}
	}
	if err := saveConfig(next); err != nil {
		saveErr := fmt.Errorf("save config: %w", err)
		if !autoStartChanged {
			return cfg, saveErr
		}
		rollbackErr := setAutoStart(currentAutoStart)
		if rollbackErr != nil {
			rollbackErr = fmt.Errorf("roll back auto-start setting: %w", rollbackErr)
		}
		return cfg, errors.Join(saveErr, rollbackErr)
	}
	return next, nil
}
