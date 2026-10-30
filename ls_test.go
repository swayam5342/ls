package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type fakeInfo struct {
	name string
	size int64
	mode fs.FileMode
}

func (f fakeInfo) Name() string       { return f.name }
func (f fakeInfo) Size() int64        { return f.size }
func (f fakeInfo) Mode() fs.FileMode  { return f.mode }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeInfo) Sys() any           { return nil }

func TestParseArgs(t *testing.T) {
	opts, operands, err := parseArgs([]string{"-laR", "dir", "--color=never", "-w", "40", "--", "-file"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.all || opts.layout != layoutLong || !opts.recursive || opts.color != colorNever || opts.width != 40 {
		t.Errorf("unexpected options: %+v", opts)
	}
	if !slices.Equal(operands, []string{"dir", "-file"}) {
		t.Errorf("operands = %q", operands)
	}

	opts, _, _ = parseArgs([]string{"-aA", "-tS", "-w20", "-l1"})
	if opts.all || !opts.almostAll || opts.sort != sortSize || opts.width != 20 || opts.layout != layoutSingle {
		t.Errorf("later flags should win: %+v", opts)
	}

	for _, bad := range [][]string{{"-z"}, {"--bogus"}, {"--color=sometimes"}, {"--all=yes"}, {"-w"}, {"-wx"}, {"--sort=age"}} {
		if _, _, err := parseArgs(bad); err == nil {
			t.Errorf("parseArgs(%q) succeeded, want error", bad)
		}
	}
}

func TestHumanSize(t *testing.T) {
	tests := map[int64]string{
		0: "0", 1023: "1023", 1024: "1.0K", 1025: "1.1K", 1536: "1.5K",
		10 * 1024: "10K", 10*1024 + 1: "11K", 1024*1024 - 1: "1.0M",
		5 << 30: "5.0G", 1 << 60: "1.0E",
	}
	for n, want := range tests {
		if got := humanSize(n); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestModeString(t *testing.T) {
	tests := []struct {
		mode fs.FileMode
		want string
	}{
		{0o644, "-rw-r--r--"},
		{fs.ModeDir | 0o755, "drwxr-xr-x"},
		{fs.ModeSymlink | 0o777, "lrwxrwxrwx"},
		{fs.ModeSetuid | 0o755, "-rwsr-xr-x"},
		{fs.ModeSetgid | 0o644, "-rw-r-Sr--"},
		{fs.ModeDir | fs.ModeSticky | 0o777, "drwxrwxrwt"},
		{fs.ModeNamedPipe | 0o600, "prw-------"},
		{fs.ModeDevice | fs.ModeCharDevice | 0o666, "crw-rw-rw-"},
	}
	for _, tt := range tests {
		if got := modeString(fakeInfo{name: "f", mode: tt.mode}); got != tt.want {
			t.Errorf("modeString(%v) = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestGridLayout(t *testing.T) {
	widths := []int{5, 5, 5, 5, 5}
	if rows, cols := gridLayout(widths, 80); rows != 1 || len(cols) != 5 {
		t.Errorf("wide terminal: rows=%d cols=%v", rows, cols)
	}
	if rows, cols := gridLayout(widths, 19); rows != 2 || len(cols) != 3 {
		t.Errorf("width 19: rows=%d cols=%v", rows, cols)
	}
	if rows, cols := gridLayout(widths, 3); rows != 5 || len(cols) != 1 {
		t.Errorf("narrow terminal: rows=%d cols=%v", rows, cols)
	}
}

func TestFormatTime(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	if got := formatTime(now.AddDate(0, -1, 0), now); got != "Oct  1 12:00" {
		t.Errorf("recent: %q", got)
	}
	if got := formatTime(now.AddDate(-1, 0, 0), now); got != "Oct 31  2025" {
		t.Errorf("old: %q", got)
	}
	if got := formatTime(now.AddDate(0, 0, 2), now); got != "Nov  2  2026" {
		t.Errorf("future: %q", got)
	}
}

func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]int{"b.txt": 3, "a.txt": 10, ".hidden": 1, "Big.log": 100, "sub/c.txt": 1}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, size := range files {
		if err := os.WriteFile(filepath.Join(dir, name), bytes.Repeat([]byte("x"), size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runLS(t *testing.T, args ...string) (stdout, stderr string, status int) {
	t.Helper()
	var out, errOut bytes.Buffer
	e := env{width: 80, now: time.Now(), getenv: func(string) string { return "" }}
	status = run(args, &out, &errOut, e)
	return out.String(), errOut.String(), status
}

func TestListing(t *testing.T) {
	dir := fixture(t)
	sep := string(os.PathSeparator)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"default", []string{dir}, "a.txt\nb.txt\nBig.log\nsub\n"},
		{"all", []string{"-a", dir}, ".\n..\n.hidden\na.txt\nb.txt\nBig.log\nsub\n"},
		{"almost all", []string{"-A", dir}, ".hidden\na.txt\nb.txt\nBig.log\nsub\n"},
		{"reverse", []string{"-r", dir}, "sub\nBig.log\nb.txt\na.txt\n"},
		{"size", []string{"-S", dir}, "Big.log\na.txt\nb.txt\nsub\n"},
		{"extension", []string{"-X", dir}, "sub\nBig.log\na.txt\nb.txt\n"},
		{"classify", []string{"-F", dir}, "a.txt\nb.txt\nBig.log\nsub/\n"},
		{"dirs first", []string{"--group-directories-first", dir}, "sub\na.txt\nb.txt\nBig.log\n"},
		{"columns", []string{"-C", "-w", "20", dir}, "a.txt  Big.log\nb.txt  sub\n"},
		{"recursive", []string{"-R", dir},
			dir + ":\na.txt\nb.txt\nBig.log\nsub\n\n" + dir + sep + "sub:\nc.txt\n"},
		{"files then dirs", []string{filepath.Join(dir, "sub"), filepath.Join(dir, "a.txt")},
			filepath.Join(dir, "a.txt") + "\n\n" + filepath.Join(dir, "sub") + ":\nc.txt\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, stderr, status := runLS(t, append([]string{"-1"}, tt.args...)...)
			if status != 0 || stderr != "" {
				t.Fatalf("status %d, stderr %q", status, stderr)
			}
			if got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestSortBySize(t *testing.T) {
	dir := fixture(t)
	got, _, _ := runLS(t, "-1S", dir)
	files := slices.DeleteFunc(strings.Fields(got), func(s string) bool { return s == "sub" })
	if want := []string{"Big.log", "a.txt", "b.txt"}; !slices.Equal(files, want) {
		t.Errorf("got %q, want %q", files, want)
	}
	got, _, _ = runLS(t, "-1Sr", dir)
	files = slices.DeleteFunc(strings.Fields(got), func(s string) bool { return s == "sub" })
	if want := []string{"b.txt", "a.txt", "Big.log"}; !slices.Equal(files, want) {
		t.Errorf("reversed: got %q, want %q", files, want)
	}
}

func TestDirectoryFlag(t *testing.T) {
	dir := fixture(t)
	got, _, _ := runLS(t, "-d", dir)
	if got != dir+"\n" {
		t.Errorf("got %q", got)
	}
}

func TestLongFormat(t *testing.T) {
	dir := fixture(t)
	got, _, status := runLS(t, "-l", dir)
	if status != 0 {
		t.Fatalf("status %d", status)
	}
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != 5 || !strings.HasPrefix(lines[0], "total ") {
		t.Fatalf("unexpected output:\n%s", got)
	}
	big := lines[3]
	if !strings.HasPrefix(big, "-") || !strings.HasSuffix(big, " Big.log") || !strings.Contains(big, " 100 ") {
		t.Errorf("Big.log line = %q", big)
	}
	if !strings.HasPrefix(lines[4], "d") || !strings.HasSuffix(lines[4], " sub") {
		t.Errorf("sub line = %q", lines[4])
	}
	if strings.Index(lines[1], "a.txt") != strings.Index(lines[3], "Big.log") {
		t.Errorf("columns not aligned:\n%s", got)
	}
}

func TestErrors(t *testing.T) {
	dir := fixture(t)
	missing := filepath.Join(dir, "nope")
	out, stderr, status := runLS(t, "-1", missing, filepath.Join(dir, "a.txt"))
	if status != 2 {
		t.Errorf("status = %d, want 2", status)
	}
	if want := "ls: cannot access '" + missing + "': No such file or directory\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	if out != filepath.Join(dir, "a.txt")+"\n" {
		t.Errorf("stdout = %q", out)
	}

	if _, stderr, status := runLS(t, "-Q"); status != 2 || !strings.Contains(stderr, "invalid option -- 'Q'") {
		t.Errorf("bad flag: status %d, stderr %q", status, stderr)
	}
}

func TestColor(t *testing.T) {
	dir := fixture(t)
	got, _, _ := runLS(t, "-1", "--color=always", dir)
	if !strings.Contains(got, "\x1b[01;34msub\x1b[0m\n") {
		t.Errorf("directory not colored: %q", got)
	}
	if !strings.HasPrefix(got, "a.txt\n") {
		t.Errorf("plain file should not be colored: %q", got)
	}

	p := newPalette("di=00;33:*.log=01;32")
	if p.dir != "00;33" || p.code(fakeInfo{name: "x.LOG"}, nil) != "01;32" || p.code(fakeInfo{name: "x.zip"}, nil) != "" {
		t.Errorf("LS_COLORS not applied: %+v", p)
	}
}

func TestSymlink(t *testing.T) {
	dir := fixture(t)
	link := filepath.Join(dir, "link")
	if err := os.Symlink("sub", link); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}
	got, _, _ := runLS(t, "-1F", dir)
	if !strings.Contains(got, "\nlink@\n") {
		t.Errorf("-F should mark links: %q", got)
	}
	got, _, _ = runLS(t, "-l", link)
	if !strings.HasPrefix(got, "l") || !strings.HasSuffix(got, " -> sub\n") {
		t.Errorf("long link = %q", got)
	}
	if got, _, _ := runLS(t, "-1", link); got != "c.txt\n" {
		t.Errorf("link operand = %q", got)
	}
}
