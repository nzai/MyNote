package config

import (
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
