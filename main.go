package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"strconv"
	"time"
)

const version = "0.1.0"

const usage = `Usage: ls [OPTION]... [FILE]...
List information about the FILEs (the current directory by default).
Entries are sorted alphabetically unless a sort option is given.

  -a, --all                  do not ignore entries starting with .
  -A, --almost-all           do not list implied . and ..
  -C                         list entries by columns
  -d, --directory            list directories themselves, not their contents
  -F, --classify             append indicator (one of */=@|) to entries
  -h, --human-readable       with -l, print sizes like 1K 234M 2G
  -l                         use a long listing format
  -p                         append / indicator to directories
  -r, --reverse              reverse order while sorting
  -R, --recursive            list subdirectories recursively
  -S                         sort by file size, largest first
  -t                         sort by modification time, newest first
  -U                         do not sort; list entries in directory order
  -w, --width=COLS           set output width to COLS; 0 means no limit
  -X                         sort alphabetically by entry extension
  -1                         list one file per line
      --color[=WHEN]         colorize the output; WHEN is 'always' (the
                             default when given without WHEN), 'auto' or
                             'never'; without the option, 'auto' is used
      --group-directories-first
                             group directories before files
      --sort=WORD            sort by WORD: none, name, size, time, extension
      --help                 display this help and exit
      --version              output version information and exit

Colors can be customized with the LS_COLORS environment variable and turned
off with NO_COLOR.

Exit status:
 0  if OK,
 1  if minor problems (e.g., cannot access subdirectory),
 2  if serious trouble (e.g., cannot access command-line argument).
`

type env struct {
	tty    bool
	width  int
	now    time.Time
	getenv func(string) string
}

func main() {
	e := env{tty: isTerminal(os.Stdout), width: 80, now: time.Now(), getenv: os.Getenv}
	if e.tty {
		enableVirtualTerminal(os.Stdout)
		if w := terminalWidth(os.Stdout); w > 0 {
			e.width = w
		}
	} else if w, err := parsePositive(os.Getenv("COLUMNS")); err == nil {
		e.width = w
	}
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, e))
}

func run(args []string, stdout, stderr io.Writer, e env) int {
	opts, operands, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "ls: %v\nTry 'ls --help' for more information.\n", err)
		return 2
	}
	if opts.help {
		io.WriteString(stdout, usage)
		return 0
	}
	if opts.version {
		fmt.Fprintf(stdout, "ls (Go clone) %s\n", version)
		return 0
	}
	if opts.width == 0 {
		e.width = math.MaxInt32
	} else if opts.width > 0 {
		e.width = opts.width
	}

	out := bufio.NewWriter(stdout)
	l := newLister(opts, out, stderr, e)
	l.listOperands(operands)
	out.Flush()
	return l.status
}

func parsePositive(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, errors.New("not a positive integer")
	}
	return n, nil
}

func errMsg(err error) string {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "No such file or directory"
	case errors.Is(err, fs.ErrPermission):
		return "Permission denied"
	}
	if pe, ok := errors.AsType[*fs.PathError](err); ok {
		return pe.Err.Error()
	}
	return err.Error()
}
