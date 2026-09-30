//go:build !windows

package main

// ownsConsole is only meaningful on Windows, where a program started from Explorer gets a
// console window of its own that closes when the program exits.
func ownsConsole() bool { return false }

func waitForKey() {}
