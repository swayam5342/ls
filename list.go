package main

import (
	"bufio"
	"cmp"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type entry struct {
	name       string
	path       string
	info       fs.FileInfo
	target     string
	targetInfo fs.FileInfo
}

func newEntry(name, path string, info fs.FileInfo) *entry {
	e := &entry{name: name, path: path, info: info}
	if e.isLink() {
		e.target, _ = os.Readlink(path)
		e.targetInfo, _ = os.Stat(path)
	}
	return e
}

func (e *entry) isLink() bool { return e.info.Mode()&fs.ModeSymlink != 0 }

func (e *entry) isDirLike() bool {
	if e.isLink() {
		return e.targetInfo != nil && e.targetInfo.IsDir()
	}
	return e.info.IsDir()
}

type lister struct {
	opts    *options
	out     *bufio.Writer
	errw    io.Writer
	env     env
	long    bool
	columns bool
	color   bool
	pal     *palette
	status  int
}

func newLister(opts *options, out *bufio.Writer, errw io.Writer, e env) *lister {
	l := &lister{opts: opts, out: out, errw: errw, env: e}
	switch opts.layout {
	case layoutLong:
		l.long = true
	case layoutColumns:
		l.columns = true
	case layoutAuto:
		l.columns = e.tty
	}
	switch opts.color {
	case colorAlways:
		l.color = true
	case colorAuto:
		l.color = e.tty && e.getenv("NO_COLOR") == "" && e.getenv("TERM") != "dumb"
	}
	if l.color {
		l.pal = newPalette(e.getenv("LS_COLORS"))
	}
	return l
}

func (l *lister) setStatus(s int) { l.status = max(l.status, s) }

func (l *lister) errorf(format string, args ...any) {
	l.out.Flush()
	fmt.Fprintf(l.errw, "ls: "+format+"\n", args...)
}

func (l *lister) fail(top bool, format string, args ...any) {
	l.errorf(format, args...)
	if top {
		l.setStatus(2)
	} else {
		l.setStatus(1)
	}
}

func (l *lister) listOperands(operands []string) {
	if len(operands) == 0 {
		operands = []string{"."}
	}
	var files, dirs []*entry
	for _, op := range operands {
		info, err := os.Lstat(op)
		if err != nil {
			l.fail(true, "cannot access '%s': %s", op, errMsg(err))
			continue
		}
		e := newEntry(op, op, info)
		if !l.opts.directory && l.followOperand(e) {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}
	l.sortEntries(files)
	l.sortEntries(dirs)

	if len(files) > 0 {
		l.printEntries(files, false)
	}
	header := len(operands) > 1 || l.opts.recursive
	for i, d := range dirs {
		if i > 0 || len(files) > 0 {
			l.out.WriteByte('\n')
		}
		l.listDir(d.path, header, true)
	}
}

func (l *lister) followOperand(e *entry) bool {
	if e.info.IsDir() {
		return true
	}
	return e.isLink() && e.isDirLike() && !l.long && !l.opts.classify
}

func (l *lister) listDir(dir string, header, top bool) {
	if header {
		fmt.Fprintf(l.out, "%s:\n", dir)
	}
	f, err := os.Open(dir)
	if err != nil {
		l.fail(top, "cannot open directory '%s': %s", dir, errMsg(err))
		return
	}
	des, err := f.ReadDir(-1)
	f.Close()
	if err != nil {
		l.fail(top, "reading directory '%s': %s", dir, errMsg(err))
	}

	entries := l.collect(dir, des)
	l.sortEntries(entries)
	l.printEntries(entries, true)

	if !l.opts.recursive {
		return
	}
	for _, e := range entries {
		if e.info.IsDir() && e.name != "." && e.name != ".." {
			l.out.WriteByte('\n')
			l.listDir(e.path, true, false)
		}
	}
}

func (l *lister) collect(dir string, des []fs.DirEntry) []*entry {
	var entries []*entry
	if l.opts.all {
		for _, name := range []string{".", ".."} {
			p := joinPath(dir, name)
			if info, err := os.Lstat(p); err == nil {
				entries = append(entries, newEntry(name, p, info))
			}
		}
	}
	for _, de := range des {
		p := joinPath(dir, de.Name())
		info, err := de.Info()
		if err != nil {
			l.fail(false, "cannot access '%s': %s", p, errMsg(err))
			continue
		}
		if !l.opts.all && !l.opts.almostAll && isHidden(de.Name(), info) {
			continue
		}
		entries = append(entries, newEntry(de.Name(), p, info))
	}
	return entries
}

func joinPath(dir, name string) string {
	if dir == "" || os.IsPathSeparator(dir[len(dir)-1]) || filepath.VolumeName(dir) == dir {
		return dir + name
	}
	return dir + string(os.PathSeparator) + name
}

func (l *lister) sortEntries(es []*entry) {
	if l.opts.sort != sortNone {
		slices.SortStableFunc(es, func(a, b *entry) int {
			if l.opts.reverse {
				return l.compare(b, a)
			}
			return l.compare(a, b)
		})
	}
	if l.opts.dirsFirst {
		slices.SortStableFunc(es, func(a, b *entry) int {
			switch da, db := a.isDirLike(), b.isDirLike(); {
			case da && !db:
				return -1
			case db && !da:
				return 1
			}
			return 0
		})
	}
}

func (l *lister) compare(a, b *entry) int {
	switch l.opts.sort {
	case sortTime:
		if c := b.info.ModTime().Compare(a.info.ModTime()); c != 0 {
			return c
		}
	case sortSize:
		if c := cmp.Compare(b.info.Size(), a.info.Size()); c != 0 {
			return c
		}
	case sortExt:
		if c := strings.Compare(extension(a.name), extension(b.name)); c != 0 {
			return c
		}
	}
	return compareNames(a.name, b.name)
}

func compareNames(a, b string) int {
	if c := strings.Compare(strings.ToLower(a), strings.ToLower(b)); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

func extension(name string) string {
	base := filepath.Base(name)
	i := strings.LastIndexByte(base, '.')
	if i <= 0 {
		return ""
	}
	return strings.ToLower(base[i+1:])
}
