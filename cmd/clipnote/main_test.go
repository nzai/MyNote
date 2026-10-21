package main

import (
	"log/slog"
	"reflect"
	"testing"

	"github.com/nzai/MyNote/internal/core"
	"github.com/nzai/MyNote/internal/save"
)

func TestMergeDebugScopes(t *testing.T) {
	got := mergeDebugScopes([]string{"cache"}, " UI, cache,clipboard ")
	want := []string{"cache", "ui", "clipboard"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeDebugScopes() = %v, want %v", got, want)
	}
}

func TestParseCLIRejectsPositionalArguments(t *testing.T) {
	if _, err := parseCLI([]string{"unexpected"}); err == nil {
		t.Fatal("expected positional argument error")
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}
	for input, want := range tests {
		got, err := parseLogLevel(input)
		if err != nil {
			t.Fatalf("parseLogLevel(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("parseLogLevel(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestNewSaveServiceUsesCapturedSnapshot(t *testing.T) {
	service := newSaveService(core.Snapshot{
		Kind: core.KindText,
		Text: "captured before edits",
	}).(*save.Service)

	if service.Kind() != core.KindText || service.Text() != "captured before edits" {
		t.Fatalf("kind = %v, text = %q", service.Kind(), service.Text())
	}
}
