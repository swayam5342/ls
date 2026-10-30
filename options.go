package main

import (
	"fmt"
	"strconv"
	"strings"
)

type sortKey int

const (
	sortName sortKey = iota
	sortTime
	sortSize
	sortExt
	sortNone
)

type colorMode int

const (
	colorAuto colorMode = iota
	colorAlways
	colorNever
)

type layout int

const (
	layoutAuto layout = iota
	layoutLong
	layoutSingle
	layoutColumns
)

type options struct {
	all       bool
	almostAll bool
	layout    layout
	human     bool
	reverse   bool
	sort      sortKey
	recursive bool
	directory bool
	classify  bool
	slash     bool
	dirsFirst bool
	color     colorMode
	width     int
	help      bool
	version   bool
}

func parseArgs(args []string) (*options, []string, error) {
	opts := &options{width: -1}
	var operands []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			return opts, append(operands, args[i+1:]...), nil
		case strings.HasPrefix(arg, "--"):
			name, value, hasValue := strings.Cut(arg[2:], "=")
			if name == "width" && !hasValue {
				if i+1 >= len(args) {
					return nil, nil, fmt.Errorf("option '--width' requires an argument")
				}
				i++
				value, hasValue = args[i], true
			}
			if err := opts.setLong(name, value, hasValue); err != nil {
				return nil, nil, err
			}
		case len(arg) > 1 && arg[0] == '-':
			for j := 1; j < len(arg); j++ {
				if arg[j] == 'w' {
					value := arg[j+1:]
					if value == "" {
						if i+1 >= len(args) {
							return nil, nil, fmt.Errorf("option requires an argument -- 'w'")
						}
						i++
						value = args[i]
					}
					if err := opts.setWidth(value); err != nil {
						return nil, nil, err
					}
					break
				}
				if err := opts.setShort(arg[j]); err != nil {
					return nil, nil, err
				}
			}
		default:
			operands = append(operands, arg)
		}
	}
	return opts, operands, nil
}

func (o *options) setShort(c byte) error {
	switch c {
	case 'a':
		o.all, o.almostAll = true, false
	case 'A':
		o.almostAll, o.all = true, false
	case 'l':
		o.layout = layoutLong
	case '1':
		o.layout = layoutSingle
	case 'C':
		o.layout = layoutColumns
	case 'h':
		o.human = true
	case 'r':
		o.reverse = true
	case 't':
		o.sort = sortTime
	case 'S':
		o.sort = sortSize
	case 'X':
		o.sort = sortExt
	case 'U':
		o.sort = sortNone
	case 'R':
		o.recursive = true
	case 'd':
		o.directory = true
	case 'F':
		o.classify = true
	case 'p':
		o.slash = true
	default:
		return fmt.Errorf("invalid option -- '%c'", c)
	}
	return nil
}

var longAliases = map[string]byte{
	"all":            'a',
	"almost-all":     'A',
	"human-readable": 'h',
	"reverse":        'r',
	"recursive":      'R',
	"directory":      'd',
	"classify":       'F',
}

func (o *options) setLong(name, value string, hasValue bool) error {
	noArg := func() error {
		if hasValue {
			return fmt.Errorf("option '--%s' doesn't allow an argument", name)
		}
		return nil
	}
	if c, ok := longAliases[name]; ok {
		if err := noArg(); err != nil {
			return err
		}
		return o.setShort(c)
	}
	switch name {
	case "group-directories-first":
		o.dirsFirst = true
		return noArg()
	case "help":
		o.help = true
		return noArg()
	case "version":
		o.version = true
		return noArg()
	case "color", "colour":
		if !hasValue {
			o.color = colorAlways
			return nil
		}
		switch value {
		case "always", "yes", "force":
			o.color = colorAlways
		case "never", "no", "none":
			o.color = colorNever
		case "auto", "tty", "if-tty":
			o.color = colorAuto
		default:
			return fmt.Errorf("invalid argument '%s' for '--%s'", value, name)
		}
	case "sort":
		keys := map[string]sortKey{"name": sortName, "none": sortNone, "size": sortSize, "time": sortTime, "extension": sortExt}
		k, ok := keys[value]
		if !ok {
			return fmt.Errorf("invalid argument '%s' for '--sort'", value)
		}
		o.sort = k
	case "width":
		return o.setWidth(value)
	default:
		return fmt.Errorf("unrecognized option '--%s'", name)
	}
	return nil
}

func (o *options) setWidth(value string) error {
	w, err := strconv.Atoi(value)
	if err != nil || w < 0 {
		return fmt.Errorf("invalid line width: '%s'", value)
	}
	o.width = w
	return nil
}
