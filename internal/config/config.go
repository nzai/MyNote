package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	defaultMaxTextBytes  int64 = 32 * 1024 * 1024
	defaultMaxImageBytes int64 = 128 * 1024 * 1024
	defaultMaxCacheBytes int64 = 500 * 1024 * 1024

	MinEditorFontPointSize = 8
	MaxEditorFontPointSize = 48
	maxEditorFontFamilyLen = 128
)

type Config struct {
	Cache   CacheConfig   `json:"cache"`
	Capture CaptureConfig `json:"capture"`
	UI      UIConfig      `json:"ui"`
	Log     LogConfig     `json:"log"`
}

type CacheConfig struct {
	MaxAgeDays int   `json:"max_age_days"`
	MaxBytes   int64 `json:"max_bytes"`
}

type CaptureConfig struct {
	MaxTextBytes  int64 `json:"max_text_bytes"`
	MaxImageBytes int64 `json:"max_image_bytes"`
}

type UIConfig struct {
	ProgressDelayMS int          `json:"progress_delay_ms"`
	Editor          EditorConfig `json:"editor"`
}

type EditorConfig struct {
	FontFamily          string `json:"font_family"`
	FontPointSize       int    `json:"font_point_size"`
	LineSpacing         int    `json:"line_spacing"`
	ShowLineNumbers     bool   `json:"show_line_numbers"`
	SuppressClosePrompt bool   `json:"suppress_close_prompt"`
}

type LogConfig struct {
	Level       string   `json:"level"`
	DebugScopes []string `json:"debug_scopes"`
}

func Default() Config {
	return Config{
		Cache: CacheConfig{
			MaxAgeDays: 7,
			MaxBytes:   defaultMaxCacheBytes,
		},
		Capture: CaptureConfig{
			MaxTextBytes:  defaultMaxTextBytes,
			MaxImageBytes: defaultMaxImageBytes,
		},
		UI: UIConfig{
			ProgressDelayMS: 3000,
			Editor:          DefaultEditorConfig(),
		},
		Log: LogConfig{
			Level:       "info",
			DebugScopes: []string{},
		},
	}
}

func DefaultEditorConfig() EditorConfig {
	return EditorConfig{
		FontFamily:      "Microsoft YaHei UI",
		FontPointSize:   11,
		LineSpacing:     125,
		ShowLineNumbers: true,
	}
}

func LineSpacingOptions() []int {
	return []int{100, 125, 150, 200}
}

func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config %q: %w", path, err)
	}
	if err := ensureJSONEOF(dec); err != nil {
		return Config{}, fmt.Errorf("decode config %q: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config %q: %w", path, err)
	}
	return cfg, nil
}

func ensureJSONEOF(dec *json.Decoder) error {
	var extra any
	err := dec.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values")
	}
	return err
}

func (c Config) Validate() error {
	if c.Cache.MaxAgeDays <= 0 {
		return errors.New("cache.max_age_days must be positive")
	}
	if c.Cache.MaxBytes <= 0 {
		return errors.New("cache.max_bytes must be positive")
	}
	if c.Capture.MaxTextBytes <= 0 {
		return errors.New("capture.max_text_bytes must be positive")
	}
	if c.Capture.MaxImageBytes <= 0 {
		return errors.New("capture.max_image_bytes must be positive")
	}
	if c.UI.ProgressDelayMS <= 0 {
		return errors.New("ui.progress_delay_ms must be positive")
	}
	if err := c.UI.Editor.Validate(); err != nil {
		return fmt.Errorf("ui.editor: %w", err)
	}

	switch strings.ToLower(c.Log.Level) {
	case "debug", "info", "warn", "error":
	default:
		return errors.New("log.level must be debug, info, warn, or error")
	}
	for _, scope := range c.Log.DebugScopes {
		if strings.TrimSpace(scope) == "" {
			return errors.New("log.debug_scopes must not contain empty values")
		}
	}
	return nil
}

func (c EditorConfig) Validate() error {
	family := strings.TrimSpace(c.FontFamily)
	if family == "" {
		return errors.New("font_family must not be empty")
	}
	if utf8.RuneCountInString(family) > maxEditorFontFamilyLen {
		return fmt.Errorf("font_family exceeds %d characters", maxEditorFontFamilyLen)
	}
	if c.FontPointSize < MinEditorFontPointSize || c.FontPointSize > MaxEditorFontPointSize {
		return fmt.Errorf(
			"font_point_size must be between %d and %d",
			MinEditorFontPointSize,
			MaxEditorFontPointSize,
		)
	}
	switch c.LineSpacing {
	case 100, 125, 150, 200:
		return nil
	default:
		return errors.New("line_spacing must be 100, 125, 150, or 200")
	}
}

func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config before save: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory %q: %w", dir, err)
	}
	temp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary config in %q: %w", dir, err)
	}
	tempPath := temp.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set temporary config permissions: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync temporary config: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary config: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace config %q: %w", path, err)
	}
	return nil
}
