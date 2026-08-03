package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/intuitionamiga/IntuitionEngine/internal/ie64archive"
	"github.com/intuitionamiga/IntuitionEngine/internal/ie64link"
	"github.com/intuitionamiga/IntuitionEngine/internal/ie64obj"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("ie64ld", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	output := fs.String("o", "a.ie64", "output flat image")
	entry := fs.String("entry", "", "entry symbol")
	mapPath := fs.String("map", "", "write link map")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "ie64ld: no input files")
		return 2
	}
	if err := validatePaths(*output, *mapPath, fs.Args()); err != nil {
		fmt.Fprintf(os.Stderr, "ie64ld: %v\n", err)
		return 1
	}
	arguments := make([]ie64link.Argument, 0, fs.NArg())
	for _, path := range fs.Args() {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ie64ld: %v\n", err)
			return 1
		}
		if len(data) >= 8 && string(data[:8]) == "!<arch>\n" {
			archive, err := ie64archive.Parse(data)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ie64ld: %s: %v\n", path, err)
				return 1
			}
			arguments = append(arguments, ie64link.Argument{Name: path, Archive: archive})
		} else {
			obj, err := ie64obj.Parse(data)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ie64ld: %s: %v\n", path, err)
				return 1
			}
			arguments = append(arguments, ie64link.Argument{Name: path, Object: obj})
		}
	}
	inputs, err := ie64link.ResolveArguments(arguments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ie64ld: %v\n", err)
		return 1
	}
	result, err := ie64link.Link(inputs, ie64link.Options{Entry: *entry})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ie64ld: %v\n", err)
		return 1
	}
	if err := replaceFile(*output, result.Image, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "ie64ld: %v\n", err)
		return 1
	}
	if *mapPath != "" {
		if err := replaceFile(*mapPath, []byte(result.Map), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "ie64ld: %v\n", err)
			return 1
		}
	}
	return 0
}

func validatePaths(output, mapPath string, inputs []string) error {
	for _, input := range inputs {
		same, err := sameResolvedPath(output, input)
		if err != nil {
			return err
		}
		if same {
			return fmt.Errorf("refusing to overwrite input %s", input)
		}
	}
	if mapPath == "" {
		return nil
	}
	same, err := sameResolvedPath(mapPath, output)
	if err != nil {
		return err
	}
	if same {
		return fmt.Errorf("map path %s collides with output %s", mapPath, output)
	}
	for _, input := range inputs {
		same, err = sameResolvedPath(mapPath, input)
		if err != nil {
			return err
		}
		if same {
			return fmt.Errorf("map path %s collides with input %s", mapPath, input)
		}
	}
	return nil
}

func sameResolvedPath(a, b string) (bool, error) {
	aPath, aInfo, err := resolvedPath(a)
	if err != nil {
		return false, err
	}
	bPath, bInfo, err := resolvedPath(b)
	if err != nil {
		return false, err
	}
	return aPath == bPath || aInfo != nil && bInfo != nil && os.SameFile(aInfo, bInfo), nil
}

func resolvedPath(path string) (string, os.FileInfo, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	info, statErr := os.Stat(abs)
	if statErr == nil {
		resolved, err := filepath.EvalSymlinks(abs)
		return resolved, info, err
	}
	if !os.IsNotExist(statErr) {
		return "", nil, statErr
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", nil, err
	}
	return filepath.Join(parent, filepath.Base(abs)), nil, nil
}

func replaceFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".ie64ld-*")
	if err != nil {
		return err
	}
	name := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	ok = true
	return nil
}
