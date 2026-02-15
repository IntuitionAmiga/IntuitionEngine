package main

import (
	"fmt"
	"runtime"
	"sort"
)

// compiledFeatures tracks build-time feature flags via init() registration.
var compiledFeatures []string

func printFeatures() {
	fmt.Printf("Intuition Engine %s\n", Version)
	fmt.Printf("  Go version: %s\n", runtime.Version())
	fmt.Printf("  OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Println()
	fmt.Println("Compiled features:")

	sort.Strings(compiledFeatures)
	for _, f := range compiledFeatures {
		fmt.Printf("  %s\n", f)
	}
	if len(compiledFeatures) == 0 {
		fmt.Println("  (none)")
	}
}
