//go:build windows

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	advapi32                       = syscall.NewLazyDLL("advapi32.dll")
	procGetNamedSecurityInfoW      = advapi32.NewProc("GetNamedSecurityInfoW")
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
	procSetConsoleMode             = kernel32.NewProc("SetConsoleMode")

	sidNames = map[string]string{}
)

const (
	seFileObject                    = 1
	ownerSecurityInformation        = 0x1
	groupSecurityInformation        = 0x2
	enableVirtualTerminalProcessing = 0x4
)

func fileOwner(path string, _ fs.FileInfo) (string, string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "?", "?"
	}
	p, err := syscall.UTF16PtrFromString(abs)
	if err != nil {
		return "?", "?"
	}
	var owner, group *syscall.SID
	var sd uintptr
	r, _, _ := procGetNamedSecurityInfoW.Call(
		uintptr(unsafe.Pointer(p)), seFileObject,
		ownerSecurityInformation|groupSecurityInformation,
		uintptr(unsafe.Pointer(&owner)), uintptr(unsafe.Pointer(&group)),
		0, 0, uintptr(unsafe.Pointer(&sd)))
	if r != 0 {
		return "?", "?"
	}
	defer syscall.LocalFree(syscall.Handle(sd))
	return sidName(owner), sidName(group)
}

func sidName(sid *syscall.SID) string {
	if sid == nil {
		return "?"
	}
	key, err := sid.String()
	if err != nil {
		return "?"
	}
	if name, ok := sidNames[key]; ok {
		return name
	}
	name := key
	if account, _, _, err := sid.LookupAccount(""); err == nil {
		name = account
	}
	sidNames[key] = name
	return name
}

func fileLinks(fs.FileInfo) uint64 { return 1 }

func fileBlocks(info fs.FileInfo) int64 { return (info.Size() + 1023) / 1024 }

func isHidden(name string, info fs.FileInfo) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	attrs, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && attrs.FileAttributes&syscall.FILE_ATTRIBUTE_HIDDEN != 0
}

var execExts = map[string]bool{".exe": true, ".com": true, ".bat": true, ".cmd": true, ".ps1": true}

func filePerm(info fs.FileInfo) fs.FileMode {
	perm := info.Mode().Perm()
	if info.Mode().IsRegular() && execExts[strings.ToLower(filepath.Ext(info.Name()))] {
		perm |= 0o111
	}
	return perm
}

func isTerminal(f *os.File) bool {
	var mode uint32
	return syscall.GetConsoleMode(syscall.Handle(f.Fd()), &mode) == nil
}

func enableVirtualTerminal(f *os.File) {
	var mode uint32
	h := syscall.Handle(f.Fd())
	if syscall.GetConsoleMode(h, &mode) == nil {
		procSetConsoleMode.Call(uintptr(h), uintptr(mode|enableVirtualTerminalProcessing))
	}
}

func terminalWidth(f *os.File) int {
	var info struct {
		size, cursor             struct{ x, y int16 }
		attributes               uint16
		left, top, right, bottom int16
		maxWindow                struct{ x, y int16 }
	}
	r, _, _ := procGetConsoleScreenBufferInfo.Call(f.Fd(), uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return 0
	}
	return int(info.right-info.left) + 1
}
