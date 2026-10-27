package main

import (
	"errors"
	"reflect"
	"testing"

	"github.com/nzai/MyNote/internal/config"
)

func TestPersistUISettingsSavesEditorAndAutoStartTogether(t *testing.T) {
	cfg := config.Default()
	editor := config.EditorConfig{
		FontFamily:      "Cascadia Mono",
		FontPointSize:   18,
		LineSpacing:     150,
		ShowLineNumbers: false,
	}
	var autoStartCalls []bool
	var saved config.Config

	next, err := persistUISettings(
		cfg,
		editor,
		true,
		false,
		func(enabled bool) error {
			autoStartCalls = append(autoStartCalls, enabled)
			return nil
		},
		func(value config.Config) error {
			saved = value
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(autoStartCalls, []bool{true}) {
		t.Fatalf("auto-start calls = %v, want [true]", autoStartCalls)
	}
	if !reflect.DeepEqual(saved.UI.Editor, editor) {
		t.Fatalf("saved editor = %#v, want %#v", saved.UI.Editor, editor)
	}
	if !reflect.DeepEqual(next.UI.Editor, editor) {
		t.Fatalf("returned editor = %#v, want %#v", next.UI.Editor, editor)
	}
}

func TestPersistUISettingsRollsBackAutoStartWhenConfigSaveFails(t *testing.T) {
	cfg := config.Default()
	editor := cfg.UI.Editor
	editor.FontPointSize = 16
	saveErr := errors.New("disk full")
	var autoStartCalls []bool

	_, err := persistUISettings(
		cfg,
		editor,
		true,
		false,
		func(enabled bool) error {
			autoStartCalls = append(autoStartCalls, enabled)
			return nil
		},
		func(config.Config) error {
			return saveErr
		},
	)
	if !errors.Is(err, saveErr) {
		t.Fatalf("error = %v, want wrapped %v", err, saveErr)
	}
	if !reflect.DeepEqual(autoStartCalls, []bool{true, false}) {
		t.Fatalf("auto-start calls = %v, want [true false] rollback", autoStartCalls)
	}
}

func TestPersistUISettingsDoesNotTouchUnchangedAutoStart(t *testing.T) {
	cfg := config.Default()
	autoStartCalls := 0
	_, err := persistUISettings(
		cfg,
		cfg.UI.Editor,
		true,
		true,
		func(bool) error {
			autoStartCalls++
			return nil
		},
		func(config.Config) error {
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if autoStartCalls != 0 {
		t.Fatalf("auto-start setter called %d times, want 0", autoStartCalls)
	}
}

func TestPersistUISettingsStopsWhenAutoStartCannotBeUpdated(t *testing.T) {
	cfg := config.Default()
	autoStartErr := errors.New("registry denied")
	saveCalls := 0

	_, err := persistUISettings(
		cfg,
		cfg.UI.Editor,
		true,
		false,
		func(bool) error {
			return autoStartErr
		},
		func(config.Config) error {
			saveCalls++
			return nil
		},
	)
	if !errors.Is(err, autoStartErr) {
		t.Fatalf("error = %v, want wrapped %v", err, autoStartErr)
	}
	if saveCalls != 0 {
		t.Fatalf("config save called %d times after auto-start failure, want 0", saveCalls)
	}
}
