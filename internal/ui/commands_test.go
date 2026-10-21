//go:build windows

package ui

import (
	"testing"

	"github.com/rodrigocfd/windigo/co"
)

func TestCommandFor(t *testing.T) {
	tests := []struct {
		name  string
		key   co.VK
		shift bool
		want  Command
	}{
		{name: "save", key: co.VK_S, want: CommandSave},
		{name: "save as", key: co.VK_S, shift: true, want: CommandSaveAs},
		{name: "close", key: co.VK_W, want: CommandClose},
		{name: "unhandled", key: co.VK_A, want: CommandNone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := CommandFor(test.key, test.shift); got != test.want {
				t.Fatalf("CommandFor(%v, %v) = %v, want %v", test.key, test.shift, got, test.want)
			}
		})
	}
}
