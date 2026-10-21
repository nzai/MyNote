package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	defaultMaxTextBytes  int64 = 32 * 1024 * 1024
	defaultMaxImageBytes int64 = 128 * 1024 * 1024
	defaultMaxCacheBytes int64 = 500 * 1024 * 1024
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
	ProgressDelayMS int `json:"progress_delay_ms"`
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
		},
		Log: LogConfig{
			Level:       "info",
			DebugScopes: []string{},
		},
	}
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
