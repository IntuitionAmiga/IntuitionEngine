package main

import (
	"fmt"
	"strconv"
	"strings"
)

// parseInt parses an m68k immediate-style integer (decimal, $hex, %binary,
// 0x-prefixed). Returns the value and an error if the text does not parse.
func parseInt(s string) (int, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0, fmt.Errorf("empty integer")
	}
	neg := false
	if t[0] == '-' {
		neg = true
		t = t[1:]
	} else if t[0] == '+' {
		t = t[1:]
	}
	var v int64
	var err error
	switch {
	case strings.HasPrefix(t, "$"):
		v, err = strconv.ParseInt(t[1:], 16, 64)
	case strings.HasPrefix(t, "0x"), strings.HasPrefix(t, "0X"):
		v, err = strconv.ParseInt(t[2:], 16, 64)
	case strings.HasPrefix(t, "%"):
		v, err = strconv.ParseInt(t[1:], 2, 64)
	default:
		v, err = strconv.ParseInt(t, 10, 64)
	}
	if err != nil {
		return 0, err
	}
	if neg {
		v = -v
	}
	return int(v), nil
}
