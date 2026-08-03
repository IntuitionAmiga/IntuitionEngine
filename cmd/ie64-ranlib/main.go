package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/intuitionamiga/IntuitionEngine/internal/ie64archive"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: ie64-ranlib archive.a")
		os.Exit(2)
	}
	data, e := os.ReadFile(os.Args[1])
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	a, e := ie64archive.Parse(data)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	data, e = ie64archive.Marshal(a.Members)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	f, e := os.CreateTemp(filepath.Dir(os.Args[1]), ".ie64-ranlib-*")
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0o644); e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	if ce := f.Close(); e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(tmp, os.Args[1])
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
