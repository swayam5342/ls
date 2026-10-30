package main

import (
	"io/fs"
	"strings"
)

type suffixColor struct{ suffix, code string }

type palette struct {
	file, dir, link, exec, pipe, sock, blk, chr, orphan string
	suffixes                                            []suffixColor
}

var defaultSuffixes = func() []suffixColor {
	groups := []struct{ code, exts string }{
		{"01;31", "tar tgz zip gz bz2 xz zst 7z rar deb rpm jar"},
		{"01;35", "jpg jpeg png gif bmp svg webp ico tif tiff mp4 mkv webm avi mov"},
		{"00;36", "mp3 flac wav ogg m4a"},
	}
	var out []suffixColor
	for _, g := range groups {
		for ext := range strings.FieldsSeq(g.exts) {
			out = append(out, suffixColor{"." + ext, g.code})
		}
	}
	return out
}()

func newPalette(lsColors string) *palette {
	p := &palette{
		dir: "01;34", link: "01;36", exec: "01;32", pipe: "40;33",
		sock: "01;35", blk: "40;33;01", chr: "40;33;01", orphan: "40;31;01",
		suffixes: defaultSuffixes,
	}
	var custom []suffixColor
	for item := range strings.SplitSeq(lsColors, ":") {
		key, code, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if suffix, isGlob := strings.CutPrefix(key, "*"); isGlob {
			custom = append(custom, suffixColor{strings.ToLower(suffix), code})
			continue
		}
		fields := map[string]*string{
			"fi": &p.file, "di": &p.dir, "ln": &p.link, "ex": &p.exec, "pi": &p.pipe,
			"so": &p.sock, "bd": &p.blk, "cd": &p.chr, "or": &p.orphan,
		}
		if f, ok := fields[key]; ok {
			*f = code
		}
	}
	if custom != nil {
		p.suffixes = custom
	}
	return p
}

func (p *palette) code(info, target fs.FileInfo) string {
	m := info.Mode()
	switch {
	case m&fs.ModeSymlink != 0:
		if target == nil {
			return p.orphan
		}
		if p.link == "target" {
			return p.code(target, nil)
		}
		return p.link
	case m.IsDir():
		return p.dir
	case m&fs.ModeNamedPipe != 0:
		return p.pipe
	case m&fs.ModeSocket != 0:
		return p.sock
	case m&fs.ModeCharDevice != 0:
		return p.chr
	case m&fs.ModeDevice != 0:
		return p.blk
	case isExecutable(info):
		return p.exec
	}
	name := strings.ToLower(info.Name())
	for _, s := range p.suffixes {
		if strings.HasSuffix(name, s.suffix) {
			return s.code
		}
	}
	return p.file
}

func (l *lister) paintCode(text, code string) string {
	if !l.color || code == "" || code == "0" || code == "00" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (l *lister) paintEntry(text string, e *entry) string {
	if !l.color {
		return text
	}
	return l.paintCode(text, l.pal.code(e.info, e.targetInfo))
}

func (l *lister) paintInfo(text string, info fs.FileInfo) string {
	if !l.color {
		return text
	}
	return l.paintCode(text, l.pal.code(info, nil))
}

func (l *lister) orphanCode() string {
	if !l.color {
		return ""
	}
	return l.pal.orphan
}
