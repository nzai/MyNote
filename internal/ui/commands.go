package ui

import "github.com/rodrigocfd/windigo/co"

type Command uint8

const (
	CommandNone Command = iota
	CommandSave
	CommandSaveAs
	CommandClose
)

type TrayCommand uint16

const (
	TrayCommandNone TrayCommand = iota
	TrayCommandOpen
	TrayCommandOpenCache
	TrayCommandClearCache
	TrayCommandExit
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
