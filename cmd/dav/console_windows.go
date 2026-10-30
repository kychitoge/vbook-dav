//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

func initTerminal() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")

	// Set Console Output Code Page to UTF-8 (CP 65001)
	setConsoleOutputCP := kernel32.NewProc("SetConsoleOutputCP")
	if setConsoleOutputCP.Find() == nil {
		setConsoleOutputCP.Call(65001)
	}

	// Enable Virtual Terminal Processing for ANSI escape codes
	setConsoleMode := kernel32.NewProc("SetConsoleMode")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	if setConsoleMode.Find() == nil && getConsoleMode.Find() == nil {
		handle := syscall.Handle(os.Stdout.Fd())
		var mode uint32
		r, _, _ := getConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode)))
		if r != 0 {
			// ENABLE_VIRTUAL_TERMINAL_PROCESSING = 0x0004
			setConsoleMode.Call(uintptr(handle), uintptr(mode|0x0004))
		}
	}
}
