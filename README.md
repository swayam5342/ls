# ls

A clone of GNU `ls` in Go, using only the standard library. Runs on Windows,
Linux, macOS and the BSDs.

```sh
go build -o ls.exe .   # or: go build -o ls .
./ls.exe -lah --group-directories-first
```

In PowerShell, `ls` is an alias for `Get-ChildItem`, so run it as `.\ls.exe`.

## Features

- Flags: `-a -A -l -h -r -t -S -X -U -R -d -F -p -1 -C -w COLS`, bundled
  (`-lah`) and mixed with operands (`ls dir -l`); `--` ends options
- Long options: `--all`, `--almost-all`, `--human-readable`, `--reverse`,
  `--recursive`, `--directory`, `--classify`, `--color[=WHEN]`,
  `--sort=WORD`, `--width=COLS`, `--group-directories-first`, `--help`,
  `--version`
- Multi-column output sized to the terminal; one entry per line when piped
- Long format with permissions, links, owner, group, size, mtime and
  symlink targets; owners come from security descriptors on Windows
- Colors with `LS_COLORS` support, disabled by `NO_COLOR` or when piped
- GNU exit codes: 0 OK, 1 minor problems, 2 serious trouble
- On Windows, files with the hidden attribute count as hidden and
  `.exe/.com/.bat/.cmd/.ps1` files count as executable

## Differences from GNU ls

- Names sort case-insensitively instead of by locale collation
- Device files show their size rather than major/minor numbers
- On Windows, the link count is always 1 and `total` is based on file size
- No quoting of names with spaces; control characters print as `?` on a
  terminal

## Tests

```sh
go test ./...
```
