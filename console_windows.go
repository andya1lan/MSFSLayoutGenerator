//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	procGetConsoleProcessList = syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleProcessList")
	procGetch                 = syscall.NewLazyDLL("msvcrt.dll").NewProc("_getch")
)

// ownsConsole reports whether this process is the only one attached to its console window,
// which is the case when it was started from Explorer (double-click or drag-and-drop) rather
// than from a terminal.
func ownsConsole() bool {
	var pids [2]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	return n == 1
}

// waitForKey blocks until a key is pressed in the console window.
func waitForKey() {
	if procGetch.Find() == nil {
		procGetch.Call()
	}
}
