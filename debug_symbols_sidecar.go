package main

import (
	"os"
	"path/filepath"
	"strings"
)

func loadELFSymbolSidecar(st *SymbolTable, cpu, imagePath string) (string, error) {
	if st == nil || imagePath == "" || strings.HasPrefix(imagePath, "\x00") {
		return "", nil
	}
	candidates := []string{imagePath + ".elf"}
	ext := filepath.Ext(imagePath)
	if ext != "" {
		candidates = append(candidates, strings.TrimSuffix(imagePath, ext)+".elf")
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		return path, st.LoadELF(cpu, path)
	}
	return "", nil
}

func loadVICELabelSidecar(st *SymbolTable, cpu, mediaPath string, base uint64) (string, error) {
	if st == nil || mediaPath == "" || strings.HasPrefix(mediaPath, "\x00") {
		return "", nil
	}
	candidates := []string{mediaPath + ".lbl"}
	ext := filepath.Ext(mediaPath)
	if ext != "" {
		candidates = append(candidates, strings.TrimSuffix(mediaPath, ext)+".lbl")
	}
	for _, path := range candidates {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		loadErr := st.LoadVICELabels(cpu, f, base)
		closeErr := f.Close()
		if loadErr == nil {
			loadErr = closeErr
		}
		return path, loadErr
	}
	return "", nil
}

func loadGuestSymbolSidecar(st *SymbolTable, cpu, guestPath string, base uint64) (string, error) {
	if st == nil || guestPath == "" || strings.HasPrefix(guestPath, "\x00") {
		return "", nil
	}
	candidates := []string{guestPath + ".iesym", guestPath + ".lbl"}
	ext := filepath.Ext(guestPath)
	if ext != "" {
		stem := strings.TrimSuffix(guestPath, ext)
		candidates = append(candidates, stem+".iesym", stem+".lbl")
	}
	seen := make(map[string]bool, len(candidates))
	for _, path := range candidates {
		if seen[path] {
			continue
		}
		seen[path] = true
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		loadErr := st.LoadVICELabels(cpu, f, base)
		closeErr := f.Close()
		if loadErr == nil {
			loadErr = closeErr
		}
		return path, loadErr
	}
	return "", nil
}
