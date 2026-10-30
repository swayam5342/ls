package main

import (
	"fmt"
	"io/fs"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const colSep = 2

func (l *lister) printEntries(entries []*entry, inDir bool) {
	switch {
	case l.long:
		l.printLong(entries, inDir)
	case l.columns:
		l.printColumns(entries)
	default:
		for _, e := range entries {
			text, _ := l.shortName(e)
			l.out.WriteString(text)
			l.out.WriteByte('\n')
		}
	}
}

func (l *lister) printColumns(entries []*entry) {
	n := len(entries)
	texts := make([]string, n)
	widths := make([]int, n)
	for i, e := range entries {
		texts[i], widths[i] = l.shortName(e)
	}
	rows, colWidths := gridLayout(widths, l.env.width)
	for r := range rows {
		for c := range colWidths {
			i := c*rows + r
			if i >= n {
				break
			}
			l.out.WriteString(texts[i])
			if i+rows < n {
				l.out.WriteString(strings.Repeat(" ", colWidths[c]-widths[i]+colSep))
			}
		}
		l.out.WriteByte('\n')
	}
}

func gridLayout(widths []int, lineWidth int) (int, []int) {
	n := len(widths)
	if n == 0 {
		return 0, nil
	}
	maxCols := max(1, (lineWidth+colSep)/(slices.Min(widths)+colSep))
	for rows := (n + maxCols - 1) / maxCols; rows < n; rows++ {
		cols := (n + rows - 1) / rows
		colWidths := make([]int, cols)
		for i, w := range widths {
			colWidths[i/rows] = max(colWidths[i/rows], w)
		}
		total := colSep * (cols - 1)
		for _, w := range colWidths {
			total += w
		}
		if total <= lineWidth {
			return rows, colWidths
		}
	}
	return n, []int{slices.Max(widths)}
}

func (l *lister) printLong(entries []*entry, inDir bool) {
	if inDir {
		var total int64
		for _, e := range entries {
			total += fileBlocks(e.info)
		}
		if l.opts.human {
			fmt.Fprintf(l.out, "total %s\n", humanSize(total*1024))
		} else {
			fmt.Fprintf(l.out, "total %d\n", total)
		}
	}

	type row struct{ mode, links, owner, group, size, time, name string }
	rows := make([]row, len(entries))
	var wLinks, wOwner, wGroup, wSize int
	for i, e := range entries {
		owner, group := fileOwner(e.path, e.info)
		r := row{
			mode:  modeString(e.info),
			links: strconv.FormatUint(fileLinks(e.info), 10),
			owner: owner,
			group: group,
			size:  l.sizeString(e.info.Size()),
			time:  formatTime(e.info.ModTime(), l.env.now),
			name:  l.longName(e),
		}
		rows[i] = r
		wLinks = max(wLinks, len(r.links))
		wOwner = max(wOwner, textWidth(r.owner))
		wGroup = max(wGroup, textWidth(r.group))
		wSize = max(wSize, len(r.size))
	}
	for _, r := range rows {
		fmt.Fprintf(l.out, "%s %*s %s %s %*s %s %s\n",
			r.mode, wLinks, r.links, padRight(r.owner, wOwner), padRight(r.group, wGroup),
			wSize, r.size, r.time, r.name)
	}
}

func (l *lister) shortName(e *entry) (string, int) {
	name := l.display(e.name)
	ind := l.indicator(e.info, false)
	return l.paintEntry(name, e) + ind, textWidth(name) + len(ind)
}

func (l *lister) longName(e *entry) string {
	s := l.paintEntry(l.display(e.name), e)
	if !e.isLink() {
		return s + l.indicator(e.info, true)
	}
	target := l.display(e.target)
	if e.targetInfo == nil {
		return s + " -> " + l.paintCode(target, l.orphanCode())
	}
	return s + " -> " + l.paintInfo(target, e.targetInfo) + l.indicator(e.targetInfo, true)
}

func (l *lister) indicator(info fs.FileInfo, long bool) string {
	m := info.Mode()
	switch {
	case m.IsDir():
		if l.opts.classify || l.opts.slash {
			return "/"
		}
	case !l.opts.classify:
	case m&fs.ModeSymlink != 0:
		if !long {
			return "@"
		}
	case m&fs.ModeNamedPipe != 0:
		return "|"
	case m&fs.ModeSocket != 0:
		return "="
	case isExecutable(info):
		return "*"
	}
	return ""
}

func (l *lister) display(name string) string {
	if !l.env.tty {
		return name
	}
	unsafe := func(r rune) bool { return r == utf8.RuneError || unicode.IsControl(r) }
	if strings.IndexFunc(name, unsafe) < 0 {
		return name
	}
	return strings.Map(func(r rune) rune {
		if unsafe(r) {
			return '?'
		}
		return r
	}, name)
}

func (l *lister) sizeString(n int64) string {
	if l.opts.human {
		return humanSize(n)
	}
	return strconv.FormatInt(n, 10)
}

func textWidth(s string) int { return utf8.RuneCountInString(s) }

func padRight(s string, w int) string {
	return s + strings.Repeat(" ", max(0, w-textWidth(s)))
}

func isExecutable(info fs.FileInfo) bool {
	return info.Mode().IsRegular() && filePerm(info)&0o111 != 0
}

func modeString(info fs.FileInfo) string {
	m := info.Mode()
	b := []byte("----------")
	switch {
	case m.IsDir():
		b[0] = 'd'
	case m&fs.ModeSymlink != 0:
		b[0] = 'l'
	case m&fs.ModeNamedPipe != 0:
		b[0] = 'p'
	case m&fs.ModeSocket != 0:
		b[0] = 's'
	case m&fs.ModeCharDevice != 0:
		b[0] = 'c'
	case m&fs.ModeDevice != 0:
		b[0] = 'b'
	}
	perm := filePerm(info)
	const rwx = "rwxrwxrwx"
	for i := range 9 {
		if perm&(1<<(8-i)) != 0 {
			b[1+i] = rwx[i]
		}
	}
	special := func(i int, set bool, lower byte) {
		if !set {
			return
		}
		if b[i] == 'x' {
			b[i] = lower
		} else {
			b[i] = lower - 'a' + 'A'
		}
	}
	special(3, m&fs.ModeSetuid != 0, 's')
	special(6, m&fs.ModeSetgid != 0, 's')
	special(9, m&fs.ModeSticky != 0, 't')
	return string(b)
}

func humanSize(n int64) string {
	if n < 1024 {
		return strconv.FormatInt(n, 10)
	}
	const units = "KMGTPE"
	v, i := float64(n)/1024, 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if v < 10 {
		if r := math.Ceil(v*10) / 10; r < 10 {
			return fmt.Sprintf("%.1f%c", r, units[i])
		}
	}
	v = math.Ceil(v)
	if v >= 1024 && i < len(units)-1 {
		return fmt.Sprintf("1.0%c", units[i+1])
	}
	return fmt.Sprintf("%.0f%c", v, units[i])
}

func formatTime(t, now time.Time) string {
	if t.After(now.AddDate(0, -6, 0)) && !t.After(now.Add(time.Minute)) {
		return t.Format("Jan _2 15:04")
	}
	return t.Format("Jan _2  2006")
}
