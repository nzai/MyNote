package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	want := Default()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestLoadRejectsNegativeRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"cache":{"max_age_days":-1}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "max_age_days") {
		t.Fatalf("error should name the invalid field: %v", err)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"unexpected":true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestLoadKeepsDefaultsForOmittedFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"cache":{"max_age_days":3}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if got.Cache.MaxAgeDays != 3 {
		t.Fatalf("max age = %d, want 3", got.Cache.MaxAgeDays)
	}
	if got.Cache.MaxBytes != Default().Cache.MaxBytes {
		t.Fatalf("max bytes = %d, want default %d", got.Cache.MaxBytes, Default().Cache.MaxBytes)
	}
}

func TestDefaultEditorSettingsAreUsable(t *testing.T) {
	editor := Default().UI.Editor
	if editor.FontFamily != "Microsoft YaHei UI" {
		t.Fatalf("font family = %q", editor.FontFamily)
	}
	if editor.FontPointSize != 11 {
		t.Fatalf("font size = %d, want 11", editor.FontPointSize)
	}
	if editor.LineSpacing != 125 {
		t.Fatalf("line spacing = %d, want 125", editor.LineSpacing)
	}
	if !editor.ShowLineNumbers {
		t.Fatal("line numbers should be enabled by default")
	}
	if editor.SuppressClosePrompt {
		t.Fatal("unsaved-close prompt should be enabled by default")
	}
}

func TestSaveAndLoadRoundTripsEditorSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := Default()
	want.UI.Editor = EditorConfig{
		FontFamily:          "Cascadia Mono",
		FontPointSize:       18,
		LineSpacing:         150,
		ShowLineNumbers:     false,
		SuppressClosePrompt: true,
	}

	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.UI.Editor, want.UI.Editor) {
		t.Fatalf("editor settings = %#v, want %#v", got.UI.Editor, want.UI.Editor)
	}
}

func TestLoadAcceptsOmittedEditorSettingsFromExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"cache":{"max_age_days":3,"max_bytes":1048576},"capture":{"max_text_bytes":1048576,"max_image_bytes":1048576},"ui":{"progress_delay_ms":3000},"log":{"level":"info","debug_scopes":[]}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.UI.Editor, Default().UI.Editor) {
		t.Fatalf("editor settings = %#v, want defaults %#v", got.UI.Editor, Default().UI.Editor)
	}
}

func TestValidateRejectsUnsupportedEditorSettings(t *testing.T) {
	tests := []struct {
		name   string
		editor EditorConfig
		field  string
	}{
		{
			name: "blank font",
			editor: EditorConfig{
				FontFamily:    " ",
				FontPointSize: 11,
				LineSpacing:   125,
			},
			field: "font_family",
		},
		{
			name: "font too small",
			editor: EditorConfig{
				FontFamily:    "Segoe UI",
				FontPointSize: 7,
				LineSpacing:   125,
			},
			field: "font_point_size",
		},
		{
			name: "font too large",
			editor: EditorConfig{
				FontFamily:    "Segoe UI",
				FontPointSize: 49,
				LineSpacing:   125,
			},
			field: "font_point_size",
		},
		{
			name: "unsupported spacing",
			editor: EditorConfig{
				FontFamily:    "Segoe UI",
				FontPointSize: 11,
				LineSpacing:   135,
			},
			field: "line_spacing",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := Default()
			cfg.UI.Editor = test.editor
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), test.field) {
				t.Fatalf("Validate() error = %v, want field %q", err, test.field)
			}
		})
	}
}

func TestSaveWritesReadableJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	if err := Save(path, Default()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("saved config is not valid JSON: %v", err)
	}
	ui, ok := decoded["ui"].(map[string]any)
	if !ok {
		t.Fatalf("saved config ui = %#v", decoded["ui"])
	}
	if _, ok := ui["editor"]; !ok {
		t.Fatalf("saved config ui has no editor settings: %#v", ui)
	}
}

func TestSaveReplacesExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	first := Default()
	if err := Save(path, first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.UI.Editor.FontPointSize = 20
	second.UI.Editor.FontFamily = "Consolas"
	if err := Save(path, second); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.UI.Editor, second.UI.Editor) {
		t.Fatalf("reloaded editor = %#v, want %#v", got.UI.Editor, second.UI.Editor)
	}
}
