package ui

import "github.com/rodrigocfd/windigo/co"

type Command uint8

const (
	CommandNone Command = iota
	CommandSave
	CommandSaveAs
	CommandClose
)

func CommandFor(key co.VK, shift bool) Command {
	switch key {
	case co.VK_S:
		if shift {
			return CommandSaveAs
		}
		return CommandSave
	case co.VK_W:
		return CommandClose
	default:
		return CommandNone
	}
}
