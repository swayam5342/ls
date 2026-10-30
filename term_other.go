//go:build unix && !(linux || darwin || freebsd || netbsd || openbsd || dragonfly)

package main

import "os"

func terminalWidth(*os.File) int { return 0 }
