package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/intuitionamiga/IntuitionEngine/internal/ie64archive"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: ie64-ar rcs archive.a [members...]")
		return 2
	}
	ops := strings.TrimPrefix(args[0], "-")
	if strings.ContainsAny(ops, "DU") || !strings.Contains(ops, "r") && !strings.Contains(ops, "s") {
		fmt.Fprintf(os.Stderr, "ie64-ar: unsupported operation %q\n", args[0])
		return 2
	}
	path := args[1]
	var old []ie64archive.Member
	if data, err := os.ReadFile(path); err == nil {
		a, e := ie64archive.Parse(data)
		if e != nil {
			fmt.Fprintf(os.Stderr, "ie64-ar: %v\n", e)
			return 1
		}
		old = a.Members
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "ie64-ar: %v\n", err)
		return 1
	}
	var replacements []ie64archive.Member
	for _, p := range args[2:] {
		data, e := os.ReadFile(p)
		if e != nil {
			fmt.Fprintf(os.Stderr, "ie64-ar: %v\n", e)
			return 1
		}
		replacements = append(replacements, ie64archive.Member{Name: filepath.Base(p), Data: data})
	}
	members := ie64archive.Replace(old, replacements)
	data, err := ie64archive.Marshal(members)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ie64-ar: %v\n", err)
		return 1
	}
	if err = replace(path, data); err != nil {
		fmt.Fprintf(os.Stderr, "ie64-ar: %v\n", err)
		return 1
	}
	return 0
}

func replace(path string, data []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".ie64-ar-*")
	if e != nil {
		return e
	}
	name := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if e = f.Chmod(0o644); e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	if ce := f.Close(); e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(name, path); e != nil {
		return e
	}
	ok = true
	return nil
}
