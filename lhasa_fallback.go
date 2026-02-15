//go:build !linux && !headless

package main

import "fmt"

func init() {
	compiledFeatures = append(compiledFeatures, "lha:unavailable")
}

func DecompressLHAFile(path string) ([]byte, error) {
	return nil, fmt.Errorf("LHA decompression requires Linux with liblhasa installed")
}

func DecompressLHAData(data []byte) ([]byte, error) {
	return nil, fmt.Errorf("LHA decompression requires Linux with liblhasa installed")
}
