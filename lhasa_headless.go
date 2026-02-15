//go:build headless

package main

import "fmt"

func init() {
	compiledFeatures = append(compiledFeatures, "lha:headless")
}

func DecompressLHAFile(path string) ([]byte, error) {
	return nil, fmt.Errorf("lha decompression unavailable in headless mode")
}

func DecompressLHAData(data []byte) ([]byte, error) {
	return nil, fmt.Errorf("lha decompression unavailable in headless mode")
}
