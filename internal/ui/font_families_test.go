//go:build windows

package ui

import (
	"reflect"
	"testing"
)

func TestNormalizeFontFamiliesFiltersVerticalDuplicatesAndOrder(t *testing.T) {
	input := []string{
		"Segoe UI",
		"@Segoe UI",
		"Arial",
		"segoe ui",
		"",
		"  Microsoft YaHei UI  ",
	}
	want := []string{"Arial", "Microsoft YaHei UI", "Segoe UI"}
	if got := normalizeFontFamilies(input); !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeFontFamilies() = %#v, want %#v", got, want)
	}
}

func TestEnsureSelectedFontFamilyIsAlwaysAvailable(t *testing.T) {
	got := ensureFontFamily([]string{"Arial", "Consolas"}, "Cascadia Mono")
	want := []string{"Arial", "Cascadia Mono", "Consolas"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ensureFontFamily() = %#v, want %#v", got, want)
	}
}
