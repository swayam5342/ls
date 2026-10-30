//go:build unix

package main

import (
	"io/fs"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"
)

var (
	userNames  = map[uint32]string{}
	groupNames = map[uint32]string{}
)

func fileOwner(_ string, info fs.FileInfo) (string, string) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "?", "?"
	}
	return lookupName(userNames, st.Uid, userByID), lookupName(groupNames, st.Gid, groupByID)
}

func userByID(id string) (string, error) {
	u, err := user.LookupId(id)
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

func groupByID(id string) (string, error) {
	g, err := user.LookupGroupId(id)
	if err != nil {
		return "", err
	}
	return g.Name, nil
}

func lookupName(cache map[uint32]string, id uint32, lookup func(string) (string, error)) string {
	if name, ok := cache[id]; ok {
		return name
	}
	name := strconv.FormatUint(uint64(id), 10)
	if n, err := lookup(name); err == nil {
		name = n
	}
	cache[id] = name
	return name
}

func fileLinks(info fs.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Nlink)
	}
	return 1
}

func fileBlocks(info fs.FileInfo) int64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return int64(st.Blocks) * 512 / 1024
	}
	return (info.Size() + 1023) / 1024
}

func isHidden(name string, _ fs.FileInfo) bool { return strings.HasPrefix(name, ".") }

func filePerm(info fs.FileInfo) fs.FileMode { return info.Mode().Perm() }

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&fs.ModeCharDevice != 0
}

func enableVirtualTerminal(*os.File) {}
